package p9fsp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/hugelgupf/p9/p9"
	config "github.com/ipfs/go-ipfs-config"
	mountinter "github.com/ipfs/go-ipfs/filesystem/mount"
	provcom "github.com/ipfs/go-ipfs/filesystem/mount/providers"
	logging "github.com/ipfs/go-log"
	gomfs "github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
)

//TODO: var tmplRoot = `/` || ${CurDrive}:\ || ...
var provlog = logging.Logger("mount/9p/provider")
var errObjectNotInitialized = errors.New("method called on uninitialized object")

const (
	tmplHome     = "IPFS_HOME"
	sun_path_len = 108
)

type p9pProvider struct {
	sync.Mutex

	// 9P transport
	maddr    multiaddr.Multiaddr
	listener manet.Listener

	// IPFS API
	namespace    mountinter.Namespace
	core         coreiface.CoreAPI
	filesAPIRoot *gomfs.Root

	// FS provider
	ctx          context.Context // when canceled, signals Server close intent
	cancel       context.CancelFunc
	serverClosed chan struct{} // [async] should block until server is closed
	serverErr    error         // [async] should be guarded by serverClosed

	// object implementation
	instances provcom.InstanceCollectionState
	resLock   provcom.ResourceLock
}

func NewProvider(ctx context.Context, namespace mountinter.Namespace, addrString string, api coreiface.CoreAPI, opts ...provcom.Option) (*p9pProvider, error) {
	settings := provcom.ParseOptions(opts...)

	if strings.HasPrefix(addrString, "/unix") { // stabilize our addr string which could contain template keys and/or be relative in some way
		var err error
		if addrString, err = stabilizeUnixPath(addrString); err != nil {
			return nil, err
		}
	}

	ma, err := multiaddr.NewMultiaddr(addrString)
	if err != nil {
		return nil, err
	}

	fsCtx, cancel := context.WithCancel(ctx)
	return &p9pProvider{
		ctx:          fsCtx,
		cancel:       cancel,
		maddr:        ma,
		core:         api,
		namespace:    namespace,
		filesAPIRoot: settings.FilesAPIRoot,
		resLock:      settings.ResourceLock,
		instances:    provcom.NewInstanceCollectionState(),
	}, nil
}

// TODO: support targets that start with `maddr://` which just creates the socket and doesn't mount
// useful for systems that don't have fuse but do have plan9port, as well as allowing remote mounting
// e.g. starting the service on a TCP port would allow you to mount the instance on another machine
func (pr *p9pProvider) Graft(target string) (mountinter.Instance, error) {
	pr.Lock()
	defer pr.Unlock()

	if pr.maddr == nil {
		return nil, errObjectNotInitialized
	}

	if pr.instances.Exists(target) {
		return nil, fmt.Errorf("%q already bound", target)
	}

	var closureErr error
	if pr.listener == nil {
		fmt.Println("spinning up 9P listener")
		// spin up a listener
		// TODO: split the socket listener from the server instance itself; e.g. break up listen() into listen()+newServer(manet.Listener)
		if err := pr.listen(); err != nil {
			return nil, err
		}

		// we spawned a listener, if the mount fails, clean it up; otherwise don't
		defer func() {
			if closureErr != nil {
				fmt.Println("error encountered, closing listener:", closureErr)
				pr.maybeCleanupListener()
			}
		}()
	}

	// TODO: sync between server instance and this mount call
	// 9p.Serve() will not return an error until we try to connect via mount
	// 1 of the 2 will return an error; return either prioritizing 9P's
	// signal to cleanup somehow on error; detach all; rm server instance; rm listener

	if err := pr.mount(target); err != nil {
		closureErr = err
		fmt.Println("failed to mount, likely the node instance doesn't have mount permissions")
		return nil, err
	}

	mi := &mountInstance{
		providerMu:             &pr.Mutex,
		providerDetachCallback: pr.detach,
		target:                 target,
	}

	if err := pr.instances.Add(target); err != nil {
		return nil, err
	}

	return mi, nil
}

func (pr *p9pProvider) detach(target string) error {
	if err := pr.instances.Remove(target); err != nil {
		return err
	}
	return pr.maybeCleanupListener()
}

func (pr *p9pProvider) Grafted(target string) bool {
	return pr.instances.Exists(target)
}

func (pr *p9pProvider) Where() []string {
	pr.Lock()
	defer pr.Unlock()
	return pr.instances.List()
}

// TODO: [review] this needs to be redone
// mount is probably beter as a function what takes in a target and optionally a maddr (not as a method, although maybe)
// PlatformMount needs to either be generalized (lol no) or moved into this package and kept 9P specific
func (pr *p9pProvider) mount(target string) error {
	// TODO: [hack] either require the multiaddr to not be encapsulated and check for it
	// or handle encapsulation somehow
	// for now this whole parsing scheme isn't very good
	comp, remainder := multiaddr.SplitFirst(pr.maddr)

	var (
		mArgs   string
		mSource string
	)

	switch comp.Protocol().Code {
	case multiaddr.P_UNIX:
		mArgs = "trans=unix"
		mSource = comp.Value()
	case multiaddr.P_IP4, multiaddr.P_IP6:
		mSource = comp.Value()
		comp, _ = multiaddr.SplitFirst(remainder)
		if comp.Protocol().Code != multiaddr.P_TCP {
			return fmt.Errorf("%q must reference a TCP port", pr.maddr)
		}
		mArgs = "port=" + comp.Value()
	default:
		return fmt.Errorf("%q is not recognized as a supported address", pr.maddr)
	}

	return mountinter.PlatformMount(mSource, target, mArgs)
}

func (pr *p9pProvider) isListening() bool {
	return pr.listener != nil
}

func (pr *p9pProvider) listen() error {
	// make sure we're not in use already
	if pr.isListening() {
		return fmt.Errorf("already started and listening on %s", pr.listener.Addr())
	}

	// scan the addr for unix sockets, removing them if they exist in the addr and file system
	if err := removeUnixSockets(pr.maddr); err != nil {
		return err
	}

	// launch the listener
	listener, err := manet.Listen(pr.maddr)
	if err != nil {
		return err
	}
	pr.listener = listener // FIXME: pr.listener is being nilled before the goroutine runs
	// this happens when the user tries to mount but doesn't have permissions to
	// so Graft kill the socket

	// construct and launch the 9P resource server
	pr.serverClosed = make(chan struct{}) // [async] conditional variable

	server, err := newServer(pr.ctx, pr.namespace, pr.core, pr.filesAPIRoot)
	if err != nil {
		return err
	}

	// launch the  resource server instance in the background until `Close` is called
	// store error on the fs object then close our syncing channel (see use in `Close`)
	go func() {
		// FIXME: see above ^
		// err := server.Serve(manet.NetListener(pr.listener))
		// HACK:
		err := server.Serve(manet.NetListener(listener))

		// [async] we expect `net.Accept` to fail when the filesystem context has been canceled (for any reason)
		if pr.ctx.Err() != nil {
			// non-'accept' ops are not expected to fail, so their error is preserved
			var opErr *net.OpError
			if errors.As(pr.serverErr, &opErr) && opErr.Op != "accept" {
				pr.serverErr = err
			}
		} else {
			// unexpected failure during operation
			pr.serverErr = err
		}

		close(pr.serverClosed)
	}()

	return nil
}

func (pr *p9pProvider) maybeCleanupListener() error {
	fmt.Println("maybe close listener?")
	if pr.instances.Length() == 0 { // don't keep the listener alive if we have no instances
		fmt.Println("maybe -> yes")
		err := pr.listener.Close()
		pr.listener = nil
		return err
	}
	fmt.Println("maybe -> no")
	return nil
}

func (pr *p9pProvider) Close() error {
	pr.Lock()
	defer pr.Unlock()

	if pr.maddr == nil { // forbidden
		return fmt.Errorf("Close called on uninitialized instance")
	}

	// synchronization between interface <-> fs server
	if pr.serverClosed != nil { // implies `listen` was called prior
		pr.listener.Close() // stop accepting new clients

		if pr.instances.Length() != 0 {
			instances := pr.instances.List()
			// provider conductor is responsible for instance management
			provlog.Warnf("Close called with active instances: %v", instances)
			for _, target := range instances {
				// we don't want to track these regardless
				if err := pr.instances.Remove(target); err != nil {
					provlog.Error(err)
				}
			}
		}

		pr.cancel()       // stop and prevent any lingering fs operations, signifies "closing" intent to 9P server implementation
		<-pr.serverClosed // wait for the server thread to set the error value
		pr.listener = nil // reset `listen` conditions
		pr.serverClosed = nil

		return nil
	}

	// otherwise we were never started to begin with; default/initial value will be returned
	return pr.serverErr
}

func newServer(ctx context.Context, namespace mountinter.Namespace, core coreiface.CoreAPI, mroot *gomfs.Root) (*p9.Server, error) {
	/* TODO: [lint]
	switch namespace {
	case mountinter.NamespaceIPFS:
		ops = append(ops, common.Logger(logging.Logger("9P/IPFS")))
		attacher = ipfs.Attacher(ctx, core, ops...)

	case mountinter.NamespacePinFS:
		ops = append(ops, common.Logger(logging.Logger("9P/PinFS")))
		attacher = pinfs.Attacher(ctx, core, ops...)

	case mountinter.NamespaceIPNS:
		ops = append(ops, common.Logger(logging.Logger("9P/IPNS")))
		attacher = ipns.Attacher(ctx, core, ops...)

	case mountinter.NamespaceKeyFS:
		ops = append(ops, common.Logger(logging.Logger("9P/KeyFS")))
		attacher = keyfs.Attacher(ctx, core, ops...)

	case mountinter.NamespaceFiles:
		ops = append(ops, common.Logger(logging.Logger("9P/FilesAPI")))
		attacher = mfs.Attacher(ctx, core, ops...)

	case mountinter.NamespaceCombined:
		ops = append(ops, common.Logger(logging.Logger("9P/Overlay")))
		attacher = overlay.Attacher(ctx, core, ops...)

	default:
		return nil, fmt.Errorf("unknown namespace: %v", namespace)
	}
	*/

	var attacher p9.Attacher

	switch namespace {
	case mountinter.NamespaceIPFS, mountinter.NamespaceIPNS:
		attacher = CoreAttacher(ctx, core, namespace)
	case mountinter.NamespacePinFS:
		attacher = PinAttacher(ctx, core)
	case mountinter.NamespaceKeyFS:
		attacher = KeyAttacher(ctx, core)
	case mountinter.NamespaceFiles:
		attacher = MutableAttacher(ctx, mroot)
	// TODO
	//case mountinter.NamespaceCombined:
	default:
		return nil, fmt.Errorf("unknown namespace: %v", namespace)
	}

	return p9.NewServer(attacher), nil
}

// TODO: multiaddr encapsulation concerns; this is just going to destroy every socket, not just ours
// it should probably just operate on the final component
func removeUnixSockets(maddr multiaddr.Multiaddr) error {
	var retErr error

	multiaddr.ForEach(maddr, func(comp multiaddr.Component) bool {
		if comp.Protocol().Code == multiaddr.P_UNIX {
			target := comp.Value()
			if runtime.GOOS == "windows" {
				target = strings.TrimLeft(target, "/")
			}
			if len(target) >= sun_path_len {
				// TODO [anyone] this type of check is platform dependant and checks+errors around it should exist in `mulitaddr` when forming the actual structure
				// e.g. on Windows 1909 and lower, this will always fail when binding
				// on Linux this can cause problems if applications are not aware of the true addr length and assume `sizeof addr <= 108`

				// FIXME: we lost our logger in the port from plugin; this shouldn't use fmt
				// logger.Warning("Unix domain socket path is at or exceeds standard length `sun_path[108]` this is likely to cause problems")
				fmt.Printf("[WARNING] Unix domain socket path %q is at or exceeds standard length `sun_path[108]` this is likely to cause problems\n", target)
			}

			// discard notexist errors
			if callErr := os.Remove(target); callErr != nil && !os.IsNotExist(callErr) {
				retErr = callErr
				return false // break out of ForEach
			}
		}
		return true // continue
	})

	return retErr
}

func stabilizeUnixPath(maString string) (string, error) {
	templateValueRepoPath, err := config.PathRoot()
	if err != nil {
		return "", err
	}

	if !filepath.IsAbs(templateValueRepoPath) { // stabilize root path
		absRepo, err := filepath.Abs(templateValueRepoPath)
		if err != nil {
			return "", err
		}
		templateValueRepoPath = absRepo
	}

	// expand templates

	// prevent template from expanding to double slashed paths like `/unix//home/...` on *nix systems
	// but allow it to expand to `/unix/C:\Users...` on the Windows platform
	templateValueRepoPath = strings.TrimPrefix(templateValueRepoPath, "/")

	// only expand documented template keys, not everything
	return os.Expand(maString, func(key string) string {
		return (map[string]string{
			tmplHome: templateValueRepoPath,
		})[key]
	}), nil
}
