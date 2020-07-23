//+build !nofuse

package fuse

import (
	"errors"
	"fmt"
	"io"
	"runtime"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"
)

// NOTE: [b7952c54-1614-45ea-a042-7cfae90c5361] cgofuse only supports ReaddirPlus on Windows
// if this ever changes (bumps libfuse from 2.8 -> 3.X+), add platform support here (and to any other tags with this UUID)
// TODO: this would be best in the fuselib itself; make a patch upstream
const canReaddirPlus bool = runtime.GOOS == "windows"

type closer func() error      // io.Closer closure wrapper
func (f closer) Close() error { return f() }

func (fp *fuseMounter) Mount(requests ...Request) <-chan host.Response {
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

func (fp *fuseMounter) mount(request Request) (binding host.Binding, err error) {
	binding.Request = request

	//TODO: just return errors on init panics
	// otherwise, assume init succeeded (implementations should panic on init error)
	// (bias towards receiving a go error in the prior constructor phase instead)

	//initChan := make(host.InitSignal)
	//fp.fuseInterface.initSignal = initChan

	hostInterface := fuselib.NewFileSystemHost(fp.fuseInterface)
	hostInterface.SetCapReaddirPlus(canReaddirPlus)
	hostInterface.SetCapCaseInsensitive(false)

	// TODO: we need proper init/exit signals
	// just pass directly for now since FUSE already had init in place
	binding.Closer, err = attachToHost(hostInterface, request)
	if err == nil {
		//err = <-initChan
		return
	}
	return
}

// TODO: update comments; interface changed
func attachToHost(hostInterface *fuselib.FileSystemHost, request Request) (instanceDetach io.Closer, err error) {
	//target, name := node.ParseFuseRequest(request)

	// cgofuse will panic before calling `nodeBinding.Init` if the fuse libraries are not found
	// or encounter some kind of fatal issue
	// we want to recover from this and return an error to the waiting channel
	// (instead of exiting the process)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				instanceDetach = nil
				switch runtime.GOOS {
				case "windows":
					if typedR, ok := r.(string); ok && typedR == "cgofuse: cannot find winfsp" {
						err = errors.New("WinFSP(http://www.secfs.net/winfsp/) is required to mount on this platform, but it was not found")
					}
				default:
					err = fmt.Errorf("cgofuse panicked while attempting to mount: %v", r)
				}
			}
		}()

		// if `Mount` returns true, we expect `nodeBinding.Init` to have been invoked
		// and return an error value to us via the channel
		// otherwise `nodeBinding.Init` was not likely invoked,
		// (typically because of a permission error that is logged to the console, but unknown to us)
		// so we provide a surrogate error value to the channel in that case
		// (because `nodeBinding.Init` will never communicate with us, and the receive below would block forever)
		if !hostInterface.Mount(request.HostPath, request.FuseArgs) {
			err = fmt.Errorf("%s: mount failed for an unknown reason", request.String())
		}
	}()

	instanceDetach = closer(func() (err error) {
		// TODO: get response from fs.Destroy somehow (channel on struct on constructor)
		// return it to the caller
		/*
						go func() {
						for err := range <-destroyChan {
					if err != nil {
						err = fmt.Errorf("%s: unmount failed: %w",name, err)
					}
						}()

				// [async] destroy returns before unmount
				// channel should be buffered, and expect a single
					if !hostInterface.Unmount() && err == <- errChan {
			// for range resp.hostChan {
						err = fmt.Errorf("%s: unmount failed for an unknown reason", name)
					}
		*/
		if !hostInterface.Unmount() {
			err = fmt.Errorf("%s: unmount failed for an unknown reason", request.String())
		}
		return
	})

	return
}
