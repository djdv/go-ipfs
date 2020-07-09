//+build !nofuse

package fuse

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"

	logging "github.com/ipfs/go-log"
)

type closer func() error      // io.Closer closure wrapper
func (f closer) Close() error { return f() }

// fuseAttacher attaches requests to the host via the FUSE API
type fuseAttacher struct {
	sync.Mutex
	log logging.EventLogger

	// FS provider
	ctx           context.Context // TODO: `Close` when canceled
	cancel        context.CancelFunc
	fuseInterface *fuseInterface // the actual interface between the host and the node that requests are bound with
}

func (fp *fuseAttacher) Attach(requests ...host.Request) <-chan host.Response {
	responses := make(chan host.Response, 1)
	if len(requests) == 0 {
		close(responses)
		return responses
	}

	fp.Lock()
	defer fp.Unlock()

	go func() {
		for _, request := range requests {
			var resp host.Response
			resp.Binding, resp.Error = fp.mount(request)
			responses <- resp
			if resp.Error != nil {
				break
			}
		}
		close(responses)
	}()

	return responses
}

func (fp *fuseAttacher) mount(request host.Request) (binding host.Binding, err error) {
	binding.Request = request

	initChan := make(InitSignal)
	fp.fuseInterface.initSignal = initChan

	hostInterface := fuselib.NewFileSystemHost(fp.fuseInterface)
	hostInterface.SetCapReaddirPlus(canReaddirPlus)
	hostInterface.SetCapCaseInsensitive(false)

	// TODO: we need proper init/exit signals
	// just pass directly for now since FUSE already had init in place
	binding.Closer, err = attachToHost(hostInterface, request, initChan)
	if err == nil {
		err = <-initChan
	}
	return
}

func ParseRequest(request host.Request) (target, name string) {
	var targetIsUNC bool
	if runtime.GOOS == "windows" {
		for _, arg := range request.Arguments {
			// NOTE: UNC requests should have an empty target
			// and are only specified in the parameters
			// we use them as the identifier in this case
			const paramPrefix = "--VolumePrefix="
			if strings.HasPrefix(arg, paramPrefix) {
				name = `\` + filepath.FromSlash(strings.TrimPrefix(arg, paramPrefix))
				targetIsUNC = true
				return
			}
		}
	}

	if !targetIsUNC {
		target = request.Target
		name = request.Target
	}
	return
}

func attachToHost(hostInterface *fuselib.FileSystemHost, request host.Request, initSignal InitSignal) (instanceDetach io.Closer, err error) {
	target, name := ParseRequest(request)

	// cgofuse will panic before calling `fuseInterface.Init` if the fuse libraries are not found
	// or encounter some kind of fatal issue
	// we want to recover from this and return an error to the waiting channel
	// (instead of exiting the process)
	go func() {
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
		}()

		// if `Mount` returns true, we expect `fuseInterface.Init` to have been invoked
		// and return an error value to us via the channel
		// otherwise `fuseInterface.Init` was not likely invoked,
		// (typically because of a permission error that is logged to the console, but unknown to us)
		// so we provide a surrogate error value to the channel in that case
		// (because `fuseInterface.Init` will never communicate with us, and the receive below would block forever)
		if !hostInterface.Mount(target, request.Arguments) {
			initSignal <- fmt.Errorf("%s: mount failed for an unknown reason", name)
		}
	}()

	// wait for the `Mount` call to succeed, fail, or panic
	if err = <-initSignal; err != nil {
		return
	}

	instanceDetach = closer(func() (err error) {
		if !hostInterface.Unmount() {
			err = fmt.Errorf("%s: unmount failed for an unknown reason", name)
		}
		return
	})

	return
}
