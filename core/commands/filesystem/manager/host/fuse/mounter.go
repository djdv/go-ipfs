//+build !nofuse

package fuse

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

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
	responses := make(chan host.Response)
	if len(requests) == 0 {
		close(responses)
		return responses
	}

	fp.Lock()
	defer fp.Unlock()

	go func() {
		for _, request := range requests {
			var resp host.Response
			resp.Binding, resp.Error = mount(fp.fuseInterface, request)
			responses <- resp
			if resp.Error != nil {
				fp.log.Error(resp.Error)
				break
			}
		}
		close(responses)
	}()

	return responses
}

func mount(fi fuselib.FileSystemInterface, request Request) (binding host.Binding, err error) {
	binding.Request = request
	binding.Closer, err = attachToHost(fi, request)
	return
}

// XXX: don't do things like this, we have to because we don't control this interface
func attachToHost(fuseFS fuselib.FileSystemInterface, request Request) (instanceDetach io.Closer, err error) {
	hostInterface := fuselib.NewFileSystemHost(fuseFS)
	hostInterface.SetCapReaddirPlus(canReaddirPlus)
	hostInterface.SetCapCaseInsensitive(false)

	ctx, cancel := context.WithCancel(context.Background())
	hostPath := request.hostTarget()

	// cgofuse will panic before calling `hostBinding.Init` if the fuse libraries are not found
	// or it encounters some kind of fatal issue
	// we want to recover from this and return that error (instead of exiting the process)
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

			cancel() // [async] coordinate err assignment with return value
		}()

		// `Mount` returns false if something is wrong
		// (typically a duplicate or permission error that is logged to the console, but unknown to us)
		// but otherwise blocks forever
		// as a result we have no formal way to synchronize here,
		// so we try to poll the host FS to see if the mount succeeded
		// but inevitably assume that if we haven't received an error from the kernel by now
		// `Mount` is running successfully
		semaphore := make(chan struct{})

		go func() {
			if !hostInterface.Mount(request.HostPath, request.FuseArgs) {
				err = fmt.Errorf("%s: mount failed for an unknown reason", request.String())
				cancel()
				close(semaphore)
			}
		}()

		const (
			pollRate    = 200 * time.Millisecond
			pollTimeout = 3 * pollRate
		)

		// poll the OS to see if `Mount` succeeded
		go func() {
			for {
				select {
				case <-semaphore:
					// `Mount` failed early, it set `err`
					return
				case <-time.After(pollRate): // this is just an early return so we don't always wait the full timeout
					if _, err := os.Stat(hostPath); err == nil {
						close(semaphore)
						return // path exists, mount succeed
					}
					// requeue
				case <-time.After(pollTimeout):
					// `Mount` hasn't panicked or returned an error yet
					// but we didn't see the target in the FS
					// best we can do is assume `Mount` is running forever (as intended)
					// `err` remains nil
					close(semaphore)
					return
				}
			}
		}()

		<-semaphore // wait for the system to respond; setting `err` or not
		if err == nil {
			// if this interface is ours, set up the close channel
			if ffs, ok := fuseFS.(*hostBinding); ok {
				ffs.destroySignal = make(fuseMountSignal) // NOTE: expect this to be nil after calling `Unmount`
			}
		}

		return
	}()

	<-ctx.Done() // [async] wait for hostInterface to finish calling `Mount`
	if err != nil {
		return
	}

	// TODO: better
	// we need to remove ourselves from the index on fs.Destroy since FUSE may call fs.Destroy without us knowing
	// (like when WinFSP receives a sigint)
	// this means piping the index delete() all the way down to the FS.Destroy
	// otherwise we double close on shutdown/unmount
	// because FUSE closed the FS, but we were still tracking it in the FS manager
	instanceDetach = closer(func() (err error) {
		// if this interface is ours, retrieve any errors from the FS instance's `Destroy` method
		if ffs, ok := fuseFS.(*hostBinding); ok {
			destroySignal := ffs.destroySignal
			if destroySignal == nil { // HACK: if this is true `fs.Destroy` got called twice
				return nil // this will happen if FUSE tells the FS to stop, then our FS manager does the same
			}

			go hostInterface.Unmount()
			for fuseErr := range destroySignal {
				/* fmt:
				/n/somewhere/ipns
					fuse: HCF requested,
					failed to publish: core is out of network juice,
					failed to close /abc: PC LOAD LETTER
				*/
				if err == nil {
					err = fmt.Errorf("%s Error:\n\t%w", hostPath, fuseErr)
				} else {
					err = fmt.Errorf("%w,\n\t%s", err, fuseErr.Error())
				}
			}
			return
		}

		// otherwise just do default behaviour
		if !hostInterface.Unmount() {
			err = fmt.Errorf("%s: unmount failed for an unknown reason", request.String())
		}

		return
	})

	return
}
