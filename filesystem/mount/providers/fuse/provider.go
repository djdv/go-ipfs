//+build !nofuse

package fuse

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	mountinter "github.com/ipfs/go-ipfs/filesystem/mount"
	provcom "github.com/ipfs/go-ipfs/filesystem/mount/providers"
	gomfs "github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

// FIXME: cgofuse has its own signal/interrupt handler
// we need to fork it to remove it and handle forcing cleanup ourselves

type fuseProvider struct {
	sync.Mutex

	// IPFS API
	namespace mountinter.Namespace
	core      coreiface.CoreAPI
	filesRoot *gomfs.Root

	// FS provider
	ctx    context.Context
	cancel context.CancelFunc

	// mount interface
	instances provcom.InstanceCollectionState
	resLock   provcom.ResourceLock
	//initSignal chan error
}

func NewProvider(ctx context.Context, namespace mountinter.Namespace, fuseargs string, api coreiface.CoreAPI, opts ...provcom.Option) (*fuseProvider, error) {
	settings := provcom.ParseOptions(opts...)

	fsCtx, cancel := context.WithCancel(ctx)
	return &fuseProvider{
		ctx:       fsCtx,
		cancel:    cancel,
		core:      api,
		namespace: namespace,
		resLock:   settings.ResourceLock,
		filesRoot: settings.FilesAPIRoot,
		instances: provcom.NewInstanceCollectionState(),
	}, nil
}

func (pr *fuseProvider) Graft(target string) (mountinter.Instance, error) {
	pr.Lock()
	defer pr.Unlock()

	if pr.instances.Exists(target) {
		return nil, fmt.Errorf("%q already bound", target)
	}

	mountHost, initSignal, err := newHost(pr.ctx, pr.namespace, pr.core, pr.filesRoot)
	if err != nil {
		return nil, err
	}

	fuseTarget, fuseOpts := fuseArgs(target, pr.namespace)
	go func() {
		// cgofuse will panic before calling fs.Init() if the fuse libraries encounter issues
		// we want to recover from this and return an error to the waiting channel
		// (instead of exiting the node process)
		defer func() {
			if r := recover(); r != nil {
				switch runtime.GOOS {
				case "windows":
					if typedR, ok := r.(string); ok && typedR == "cgofuse: cannot find winfsp" {
						initSignal <- errors.New("WinFSP(http://www.secfs.net/winfsp/) is required to mount on this platform, but it was not found")
					}
				default:
					initSignal <- fmt.Errorf("cgofuse panicked while attempting to mount: %v", r)
				}
			}
			// if we didn't panic, fs.Init() was invoked properly
			// and will return an error value itself
			// (so don't send anything if the panic handler returns nil)
		}()

		if !mountHost.Mount(fuseTarget, fuseOpts) {
			initSignal <- errors.New("mount failed for an unexpected reason")
		}
	}()

	if err = <-initSignal; err != nil {
		return nil, err
	}

	mi := &mountInstance{
		providerMu:             &pr.Mutex,
		providerDetachCallback: pr.instances.Remove,
		host:                   mountHost,
		target:                 target,
	}

	if err = pr.instances.Add(target); err != nil {
		// TODO: we should probably unmount here
		return nil, err
	}

	return mi, nil
}
func (pr *fuseProvider) Grafted(target string) bool {
	return pr.instances.Exists(target)
}

func newHost(ctx context.Context, namespace mountinter.Namespace, core coreiface.CoreAPI, mroot *gomfs.Root) (*fuselib.FileSystemHost, chan error, error) {
	var (
		fsh        *fuselib.FileSystemHost
		fs         fuselib.FileSystemInterface
		initSignal = make(InitSignal)
		systemOpts = []SystemOption{
			WithInitSignal(initSignal),
			// TODO: exit signal (for foreground)
			// TODO: WithResourceLock(options.resourceLock), pass in from caller
		}
	)

	switch namespace {
	default:
		return nil, nil, fmt.Errorf("unknown namespace: %v", namespace)

	case mountinter.NamespaceCombined:
		/* TODO
		oOps := []overlay.Option{overlay.WithCommon(commonOpts...)}
		if mroot != nil {
			oOps = append(oOps, overlay.WithMFSRoot(*mroot))
		}

		fs = overlay.NewFileSystem(ctx, core, oOps...)
		*/

	case mountinter.NamespacePinFS:
		fs = NewPinFileSystem(ctx, core, systemOpts...)

	case mountinter.NamespaceKeyFS:
		fs = NewKeyFileSystem(ctx, core, systemOpts...)

	case mountinter.NamespaceIPFS, mountinter.NamespaceIPNS:
		fs = NewCoreFileSystem(ctx, core, namespace, systemOpts...)

	case mountinter.NamespaceFiles:
		if mroot == nil {
			return nil, nil, fmt.Errorf("MFS root was not provided")
		}
		fs = NewMutableFileSystem(ctx, mroot, systemOpts...)
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
