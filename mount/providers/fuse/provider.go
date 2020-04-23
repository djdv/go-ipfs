package mountfuse

import (
	"context"
	"errors"
	"fmt"
	"sync"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	ipfscore "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/core"
	"github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/ipfs"
	"github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/ipns"
	"github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/overlay"
	mountcom "github.com/ipfs/go-ipfs/mount/utils/common"
	gomfs "github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

// FIXME: cgofuse has its own signal/interrupt handler
// we need to fork it to remove it and handle forcing cleanup ourselve

type fuseProvider struct {
	sync.Mutex

	// IPFS API
	namespace mountinter.Namespace
	core      coreiface.CoreAPI
	filesRoot *gomfs.Root

	// FS provider
	ctx       context.Context
	cancel    context.CancelFunc
	closed    chan struct{}
	serverErr error

	// mount interface
	instances mountcom.InstanceCollectionState
	resLock   mountcom.ResourceLock
	//initSignal chan error
}

func NewProvider(ctx context.Context, namespace mountinter.Namespace, fuseargs string, api coreiface.CoreAPI, opts ...provcom.Option) (*fuseProvider, error) {
	options := provcom.ParseOptions(opts...)

	if options.ResourceLock == nil {
		options.ResourceLock = mountcom.NewResourceLocker()
	}

	fsCtx, cancel := context.WithCancel(ctx)
	return &fuseProvider{
		ctx:       fsCtx,
		cancel:    cancel,
		core:      api,
		namespace: namespace,
		resLock:   options.ResourceLock,
		filesRoot: options.FilesAPIRoot,
		instances: mountcom.NewInstanceCollectionState(),
	}, nil
}

// NOTE: keep return statements bare
// the named error value is accessable to a defer statement
// which is used to modify it post-return by a panic handler in certain circumstances
func (pr *fuseProvider) Graft(target string) (mi mountinter.Instance, retErr error) {
	pr.Lock()
	defer pr.Unlock()

	if pr.instances.Exists(target) {
		retErr = fmt.Errorf("%q already bound", target)
		return
	}

	mountHost, initSignal, err := newHost(pr.ctx, pr.namespace, pr.core, pr.filesRoot)
	if err != nil {
		retErr = err
		return
	}

	// cgofuse will panic on fs.Init() if the required libraries are not found
	// we want to recover from this; assigning something useful to the return error
	defer cgofuseRecover(&retErr)

	fuseTarget, fuseOpts := fuseArgs(target, pr.namespace)
	go func() {
		// NOTE: mount will either fail or panic on fuselib issues
		// if it doesn't it will call fs.Init() which will return either an error or nil
		// as a result we expect to get 0 results on the channel for panic
		// 1 result if mount failed from this goroutine or
		// 1 result from fs.Init() if mount did not fail immediately
		if !mountHost.Mount(fuseTarget, fuseOpts) {
			initSignal <- errors.New("mount failed for an unexpected reason")
		}
	}()

	if err = <-initSignal; err != nil {
		retErr = err
		return
	}

	// returned value
	mi = &mountInstance{
		providerMu:             &pr.Mutex,
		providerDetachCallback: pr.instances.Remove,
		host:                   mountHost,
		target:                 target,
	}

	if err = pr.instances.Add(target); err != nil {
		retErr = err
		return
	}

	return
}
func (pr *fuseProvider) Grafted(target string) bool {
	return pr.instances.Exists(target)
}

func newHost(ctx context.Context, namespace mountinter.Namespace, core coreiface.CoreAPI, mroot *gomfs.Root) (*fuselib.FileSystemHost, chan error, error) {
	var (
		fsh        *fuselib.FileSystemHost
		fs         fuselib.FileSystemInterface
		initSignal = make(chan error)
	)

	switch namespace {
	default:
		return nil, nil, fmt.Errorf("unknown namespace: %v", namespace)
	case mountinter.NamespaceAllInOne:
		oOps := []overlay.Option{
			overlay.WithInitSignal(initSignal),
			//overlay.WithMFSRoot(*mroot), // FIXME: unchecked pointer
		}
		fs = overlay.NewFileSystem(ctx, core, oOps...)

	case mountinter.NamespaceIPFS:
		fs = ipfs.NewFileSystem(ctx, core, ipfscore.WithInitSignal(initSignal))
	case mountinter.NamespaceIPNS:
		fs = ipns.NewFileSystem(ctx, core, ipfscore.WithInitSignal(initSignal))
	case mountinter.NamespaceFiles:
		return nil, nil, fmt.Errorf("not implemented yet: %v", namespace)
	}

	fsh = fuselib.NewFileSystemHost(fs)

	fsh.SetCapReaddirPlus(provcom.CanReaddirPlus)
	fsh.SetCapCaseInsensitive(false)

	return fsh, initSignal, nil
}

func (pr *fuseProvider) Where() []string {
	pr.Lock()
	defer pr.Unlock()
	return pr.instances.List()
}
