//+build !nofuse

package fuse

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/ipfs/go-ipfs/filesystem/mount"
	provcom "github.com/ipfs/go-ipfs/filesystem/mount/providers"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

// FIXME: cgofuse has its own signal/interrupt handler
// we need to fork it to remove it and handle forcing cleanup ourselves

const fuseOptSeperator = string(0x1F) // ASCII unit separator

type closer func() error      // io.Closer closure wrapper
func (f closer) Close() error { return f() }

// the file system instance provider
type fuseProvider struct {
	sync.Mutex
	log logging.EventLogger
	// TODO: this concept still needs to be discussed
	// it's here just for plumbing; when it becomes real the fixtures will already be in place
	resLock provcom.ResourceLock

	// FS provider
	ctx        context.Context // TODO: `Close` when canceled
	cancel     context.CancelFunc
	fs         fuselib.FileSystemInterface // the actual system we provide, as an in memory interface
	initSignal InitSignal                  // TODO: pair: signals.Entry, signals.Exit
	instances  provcom.InstanceCollection  // active instances we've provided
}

func NewProvider(ctx context.Context, namespace mount.Namespace, core coreiface.CoreAPI, opts ...provcom.Option) (mount.Provider, error) {
	opts = provcom.MaybeAppendLog(opts, LogGroup)
	settings := provcom.ParseOptions(opts...)

	// TODO: InitSignal needs to become a pair and names need to be changed
	// the option should provide 2 channels, 1 for init/open, and 1 for destroy/close
	// so that the FS can signal when it's done starting and stopping, as well as provide the context of those ops
	// line semantics:
	// init line should be restricted to (m)exclusive access, half-duplex
	// e.g. 1 instantiation means there will be 1 expected reader/writer of a single message at a time
	// no possability of cross talk should be allowed
	initSignal := make(InitSignal)
	systemOpts := []SystemOption{WithInitSignal(initSignal)}
	// TODO: WithResourceLock(options.resourceLock) when that's implemented

	// construct the system we're expected to provide
	var fs fuselib.FileSystemInterface
	switch namespace {
	case mount.NamespacePinFS:
		fs = NewPinFileSystem(ctx, core, systemOpts...)
	case mount.NamespaceKeyFS:
		fs = NewKeyFileSystem(ctx, core, systemOpts...)
	case mount.NamespaceIPFS, mount.NamespaceIPNS:
		fs = NewCoreFileSystem(ctx, core, namespace, systemOpts...)
	case mount.NamespaceFiles:
		if settings.FilesAPIRoot == nil {
			// TODO: decide who's responsibility this check is
			// it might be better to make New return an error, or even panic up front
			// right now[09ef2b4a1] it will panic on operation if the constructor receives nil
			return nil, fmt.Errorf("MFS root was not provided")
		}
		fs = NewMutableFileSystem(ctx, settings.FilesAPIRoot, systemOpts...)
	case mount.NamespaceCombined:
		/* TODO
		oOps := []overlay.Option{overlay.WithCommon(commonOpts...)}
		if mroot != nil {
			oOps = append(oOps, overlay.WithMFSRoot(*mroot))
		}
		fs = overlay.NewFileSystem(ctx, core, oOps...)
		*/
	default:
		return nil, fmt.Errorf("unknown namespace: %v", namespace)
	}

	fsCtx, cancel := context.WithCancel(ctx)
	return &fuseProvider{
		log:        settings.Log,
		ctx:        fsCtx,
		cancel:     cancel,
		fs:         fs,
		resLock:    settings.ResourceLock,
		instances:  provcom.NewInstanceCollection(),
		initSignal: initSignal,
	}, nil
}

func (fp *fuseProvider) Bind(requests ...mount.Request) error {
	if len(requests) == 0 {
		return nil
	}

	fp.Lock()
	defer fp.Unlock()

	var (
		err           error
		instanceStack = provcom.NewInstanceStack(len(requests))
	)
	defer instanceStack.Clear()

	for _, req := range requests {
		if fp.instances.Exists(req.Target) {
			err = fmt.Errorf("already bound")
			break
		}

		mountHost := fuselib.NewFileSystemHost(fp.fs)
		mountHost.SetCapReaddirPlus(provcom.CanReaddirPlus)
		mountHost.SetCapCaseInsensitive(false)

		fuseOpts := strings.Split(req.Parameter, fuseOptSeperator)
		if len(fuseOpts) == 1 && fuseOpts[0] == "" {
			fuseOpts = nil // this must be explicit for no args
		}

		go func() {
			// cgofuse will panic before calling `fs.Init` if the fuse libraries are not found
			// or encounter some kind of fatal issue
			// we want to recover from this and return an error to the waiting channel
			// (instead of exiting the process)
			defer func() {
				if r := recover(); r != nil {
					switch runtime.GOOS {
					case "windows":
						if typedR, ok := r.(string); ok && typedR == "cgofuse: cannot find winfsp" {
							fp.initSignal <- errors.New("WinFSP(http://www.secfs.net/winfsp/) is required to mount on this platform, but it was not found")
						}
					default:
						fp.initSignal <- fmt.Errorf("cgofuse panicked while attempting to mount: %v", r)
					}
				}
			}()

			// if `Mount` returns true, we expect `fs.Init` to have been invoked
			// and return an error value to us via the channel
			// otherwise `fs.Init` was not likely invoked,
			// (typically because of a permission error that is logged to the console, but unknown to us)
			// so we provide a surrogate error value to the channel in that case
			// (because `fs.Init` will never communicate with us, and the receive below would block forever)
			if !mountHost.Mount(req.Target, fuseOpts) {
				fp.initSignal <- errors.New("mount failed for an unknown reason")
			}
		}()

		// wait for the `Mount` call to succeed, fail, or panic
		if err = <-fp.initSignal; err != nil {
			break
		}

		instanceStack.Push(req, closer(func() error {
			if !mountHost.Unmount() {
				return fmt.Errorf("unspecified failure to unmount")
			}
			return nil
		}))
		requests = requests[1:] // shift successful requests out of the slice
	}

	// TODO: expose the base error values somewhere so the wrapping actually matters
	// also consistency check on inserting the target; this should be done as high up as possible
	// so it doesn't end up in the message multiple times
	if err != nil {
		failedRequest := requests[0]
		err = fmt.Errorf(
			"failed to bind %q{fuse options: %q}<->%q: %w",
			failedRequest.Namespace, strings.ReplaceAll(failedRequest.Parameter, fuseOptSeperator, " "),
			failedRequest.Target, err,
		)

		if instanceStack.Length() == 0 {
			fp.log.Error(err)
		} else {
			fp.log.Errorf("%s; attempting to detach previous targets", err)
			if uErr := instanceStack.Unwind(); uErr != nil {
				fp.log.Error(uErr)
				err = fmt.Errorf("%w; %s", err, uErr)
			}
		}
		return err
	}

	fp.instances.Add(instanceStack)
	return nil
}

func (fp *fuseProvider) List() []mount.Request {
	fp.Lock()
	defer fp.Unlock()
	return fp.instances.List()
}

func (fp *fuseProvider) Detach(requests ...mount.Request) error {
	fp.Lock()
	defer fp.Unlock()
	return fp.instances.Detach(requests...)
}

func (fp *fuseProvider) Close() error {
	fp.Lock()
	defer fp.Unlock()
	return fp.instances.Close()
}
