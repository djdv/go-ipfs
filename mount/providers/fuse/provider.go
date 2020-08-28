package mountfuse

import (
	"context"
	"errors"
	"fmt"
	"sync"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	prov "github.com/ipfs/go-ipfs/mount/providers"
	fusecommon "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	"github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/ipfs"
	"github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/ipns"
	"github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/mfs"
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
	//initSignal chan error
	// TODO: resource lock goes here
}

func NewProvider(ctx context.Context, namespace mountinter.Namespace, fuseargs string, api coreiface.CoreAPI, ops ...prov.Option) (*fuseProvider, error) {
	opts := prov.ParseOptions(ops...)

	fsCtx, cancel := context.WithCancel(ctx)
	return &fuseProvider{
		ctx:    fsCtx,
		cancel: cancel,
		//initSignal: make(chan error),
		core:      api,
		namespace: namespace,
		filesRoot: opts.FilesRoot,
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
		initSignal = make(chan error)
	)

	switch namespace {
	default:
		return nil, nil, fmt.Errorf("unknown namespace: %v", namespace)
	case mountinter.NamespaceIPFS:
		fsh = fuselib.NewFileSystemHost(&ipfs.Filesystem{
			FUSEBase: fusecommon.FUSEBase{
				Core:       core,
				FilesRoot:  mroot,
				InitSignal: initSignal,
				Ctx:        ctx,
			},
		})
	case mountinter.NamespaceIPNS:
		fsh = fuselib.NewFileSystemHost(&ipns.Filesystem{
			FUSEBase: fusecommon.FUSEBase{
				Core:       core,
				FilesRoot:  mroot,
				InitSignal: initSignal,
				Ctx:        ctx,
			},
		})
	case mountinter.NamespaceFiles:
		fsh = fuselib.NewFileSystemHost(&mfs.Filesystem{
			FUSEBase: fusecommon.FUSEBase{
				Core:       core,
				FilesRoot:  mroot,
				InitSignal: initSignal,
				Ctx:        ctx,
			},
		})
	}

	//TODO: fsh.SetCapReaddirPlus(true)
	fsh.SetCapCaseInsensitive(false)

	return fsh, initSignal, nil
}

func (pr *fuseProvider) Where() []string {
	pr.Lock()
	defer pr.Unlock()
	return pr.instances.List()
}
