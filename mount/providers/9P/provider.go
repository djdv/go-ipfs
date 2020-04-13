package mount9p

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/hugelgupf/p9/p9"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	"github.com/ipfs/go-ipfs/mount/providers/9P/filesystems/keyfs"
	"github.com/ipfs/go-ipfs/mount/providers/9P/filesystems/mfs"
	"github.com/ipfs/go-ipfs/mount/providers/9P/filesystems/overlay"
	"github.com/ipfs/go-ipfs/mount/providers/9P/filesystems/pinfs"
	"github.com/ipfs/go-ipfs/mount/providers/9P/meta"
	provops "github.com/ipfs/go-ipfs/mount/providers/options"
	mountcom "github.com/ipfs/go-ipfs/mount/utils/common"
	mountsys "github.com/ipfs/go-ipfs/mount/utils/sys"
	logging "github.com/ipfs/go-log"
	gomfs "github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
)

var provlog = logging.Logger("mount/9p/provider")

type p9pProvider struct {
	sync.Mutex

	// 9P transport
	maddr    multiaddr.Multiaddr
	listener manet.Listener

	// IPFS API
	namespace mountinter.Namespace
	core      coreiface.CoreAPI
	filesRoot *gomfs.Root

	// FS provider
	ctx          context.Context // when canceled, signals Server close intent
	cancel       context.CancelFunc
	serverClosed chan struct{} // [async] should block until server is closed
	serverErr    error         // [async] should be guarded by serverClosed

	// object implementation
	instances mountcom.InstanceCollectionState
	// TODO: resource lock goes here
}

func NewProvider(ctx context.Context, namespace mountinter.Namespace, addrString string, api coreiface.CoreAPI, ops ...provops.Option) (*p9pProvider, error) {
	opts := provops.ParseOptions(ops...)

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
		ctx:       fsCtx,
		cancel:    cancel,
		maddr:     ma,
		core:      api,
		namespace: namespace,
		filesRoot: opts.FilesRoot,
		instances: mountcom.NewInstanceCollectionState(),
	}, nil
}

func (pr *p9pProvider) Graft(target string) (mountinter.Instance, error) {
	pr.Lock()
	defer pr.Unlock()

	if pr.maddr == nil {
		return nil, errObjectNotInitalized
	}

	if pr.instances.Exists(target) {
		return nil, fmt.Errorf("%q already bound", target)
	}

	var closureErr error
	if pr.listener == nil {
		// spin up a listener
		// TODO: split the socket listener from the server instance itself; e.g. break up listen() into listen()+newServer(manet.Listener)
		if err := pr.listen(); err != nil {
			return nil, err
		}

		// we spawned a listener, if the mount fails, clean it up; otherwise don't
		defer func() {
			if closureErr != nil {
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
		return nil, err
	}

	mi := &mountInstance{
		providerMu:             &pr.Mutex,
		providerDetachCallback: pr.detach,
		target:                 target,
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

func (pr *p9pProvider) mount(target string) error {
	// TODO: either require the multiaddr to not be encapsulated and check for it
	// or handle encapsulation somehow
	// for now this isn't very good
	comp, _ := multiaddr.SplitFirst(pr.maddr)

	var (
		mArgs   string
		mSource string
	)

	switch comp.Protocol().Code {
	case multiaddr.P_UNIX:
		mArgs = "trans=unix"
		mSource = comp.Value()
	case multiaddr.P_TCP:
		mArgs = "port=" + comp.Value()
		mSource = comp.String()
	}

	return mountsys.PlatformMount(mSource, target, mArgs)
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
	pr.listener = listener

	// construct and launch the 9P resource server
	pr.serverClosed = make(chan struct{}) // [async] conditional variable

	server, err := newServer(pr.ctx, pr.namespace, pr.core, pr.filesRoot)
	if err != nil {
		return err
	}

	// launch the  resource server instance in the background until `Close` is called
	// store error on the fs object then close our syncing channel (see use in `Close`)
	go func() {
		err := server.Serve(manet.NetListener(pr.listener))

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
	if pr.instances.Length() == 0 { // don't keep the listener alive if we have no instances
		err := pr.listener.Close()
		pr.listener = nil
		return err
	}
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
	ops := []meta.AttachOption{
		meta.MFSRoot(mroot),
	}
	var attacher p9.Attacher

	switch namespace {
	case mountinter.NamespaceIPFS:
		ops = append(ops, meta.Logger(logging.Logger("9IPFS")))
		attacher = pinfs.Attacher(ctx, core, ops...)

	case mountinter.NamespaceIPNS:
		ops = append(ops, meta.Logger(logging.Logger("9IPNS")))
		attacher = keyfs.Attacher(ctx, core, ops...)

	case mountinter.NamespaceFiles:
		ops = append(ops, meta.Logger(logging.Logger("9Files")))
		attacher = mfs.Attacher(ctx, core, ops...)

	case mountinter.NamespaceAllInOne:
		ops = append(ops, meta.Logger(logging.Logger("9overlay")))
		attacher = overlay.Attacher(ctx, core, ops...)

	default:
		return nil, fmt.Errorf("unknown namespace: %v", namespace)
	}

	return p9.NewServer(attacher), nil
}
