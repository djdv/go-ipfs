package p9fsp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	ninelib "github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/9p/sys"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
)

func mountListener(target string, addr net.Addr) error {
	var (
		mArgs   string
		mSource string
	)

	switch network := addr.Network(); network {
	case "unix":
		mSource = addr.String()
		mArgs = "trans=unix"
	case "tcp":
		netHost, netPort, err := net.SplitHostPort(addr.String())
		if err != nil {
			return err
		}
		mSource = netHost
		mArgs = "port=" + netPort
	default:
		return fmt.Errorf("%q is not a supported protocol", network)
	}

	return sys.Attach(mSource, target, mArgs)
}

func listen(ctx context.Context, maddr string, server *ninelib.Server) (serverRef, error) {
	// parse and listen on the address
	ma, err := multiaddr.NewMultiaddr(maddr)
	if err != nil {
		return serverRef{}, err
	}

	mListener, err := manet.Listen(ma)
	if err != nil {
		return serverRef{}, err
	}

	// construct the actual reference
	// NOTE: [async]
	// `srvErr` will be set only once
	// The `err` function checks a "was set" boolean to assure the `error` is fully assigned, before trying to return it
	// This is because `ref.err` will be called without synchronization, and could cause a read/write collision on an `error` type
	// We don't have to care about a bool's value being fully written or not, but a partially written `error` is an node with an arbitrary value
	// `decRef` has synchronization, so it may use `srvErr` directly (after syncing)
	// The counter however, will only ever be manipulated while the reference table is in a locked state
	// (if this changes, the counter should be made atomic)
	var (
		srvErr       error
		srvErrWasSet bool
		srvWg        sync.WaitGroup // done when the server has stopped serving
		count        uint
	)

	serverRef := serverRef{
		Listener: mListener,
		incRef:   func() { count++ },
		err: func() error {
			if srvErrWasSet {
				return srvErr
			}
			return nil
		},
		decRef: func() error {
			count--
			if count == 0 {
				lstErr := mListener.Close() // will trigger the server to stop
				srvWg.Wait()                // wait for the server to assign its error

				if srvErr == nil && lstErr != nil { // server didn't encounter an error, but the listener did
					return lstErr
				}

				err := srvErr      // server encountered an error
				if lstErr != nil { // append the listener error if it encountered one too
					err = fmt.Errorf("%w; additionally the listener encountered an error on `Close`: %s", err, lstErr)
				}

				return err
			}
			return nil
		},
	}

	// launch the  resource server instance in the background
	// until either an error is encountered, or the listener is closed (which forces an "accept" error)
	srvWg.Add(1)
	go func() {
		defer srvWg.Done()
		if err := server.Serve(manet.NetListener(mListener)); err != nil {
			if ctx.Err() != nil {
				var opErr *net.OpError
				if errors.As(err, &opErr) && opErr.Op != "accept" {
					err = nil // drop this since it's expected in this case
				}
				// note that we don't filter "accept" errors when the context has not been canceled
				// as that is not expected to happen
			}
			srvErr = err
			srvErrWasSet = true // async shenanigans; see note on declaration
		}
	}()

	return serverRef, nil
}

type closer func() error      // io.Closer closure wrapper
func (f closer) Close() error { return f() }

func (pr *nineAttacher) Attach(requests ...Request) <-chan host.Response {
	responses := make(chan host.Response)
	if len(requests) == 0 {
		close(responses)
		return responses
	}

	pr.Lock()
	defer pr.Unlock()
	/* TODO: move unwind logic up to the manager
	var instanceStack = host.NewInstanceStack(len(requests))

	go func() {
		defer pr.Unlock()
		defer close(responses)
		for _, Request := range requests { // for each Request
			resp := host.Response{Binding: host.Binding{HostRequest: Request}}
			bind, err := pr.bind(Request) // try to bind
			if err != nil {               // if we can't
				resp.Err = err // alert the caller
				responses <- resp
				// undoing the previously processed requests (if any)
				for msg := range instanceStack.Unwind() {
					responses <- msg
				}
				return
			}
			instanceStack.Push(bind) // bind succeeded, push to transaction stack
		}
		pr.PathInstanceIndex.Add(instanceStack) // all binds succeeded, commit stack to index
	}()
	*/
	go func() {
		defer close(responses)
		for _, request := range requests { // for each Request
			resp := host.Response{Binding: host.Binding{Request: request}}
			resp.Binding, resp.Error = pr.bind(request)
			responses <- resp
			if resp.Error != nil {
				return
			}
		}
	}()

	return responses
}

func (pr *nineAttacher) bind(request Request) (host.Binding, error) {
	binding := host.Binding{Request: request}

	server, err := pr.getServer(request.ListenAddr)
	if err != nil {
		return binding, err
	}

	server.incRef()
	binding.Closer = closer(server.decRef)

	if request.HostPath == "" {
		return binding, nil
	}

	// otherwise, try to mount the target via a client connection to the server
	err = mountListener(request.HostPath, server.Listener.Addr())
	if err != nil {
		if sErr := server.decRef(); sErr != nil {
			err = fmt.Errorf("%w; additionally the server encountered an error on `Close`: %s", err, sErr)
		}
	}

	// in addition to closing and returning the socket error
	// make sure to detach from the host first
	socketCloser := binding.Closer.Close
	binding.Closer = closer(func() (err error) {
		hostError := sys.Detach(request.HostPath)
		sockErr := socketCloser()

		switch {
		case hostError != nil && sockErr != nil: // wrap socket error in host error
			err = fmt.Errorf("%s:%w", hostError, sockErr)
		case hostError == nil && sockErr != nil:
			err = sockErr
		case hostError != nil && sockErr == nil:
			err = hostError
		}

		return
	})

	return binding, err
}

func (pr *nineAttacher) getServer(listenAddr string) (server serverRef, err error) {
	var ok bool
	if server, ok = pr.servers[listenAddr]; ok {
		return
	}

	server, err = listen(pr.srvCtx, listenAddr, pr.srv)
	if err != nil {
		return
	}
	pr.servers[listenAddr] = server
	return
}

type serverRef struct {
	manet.Listener // socket the server is using
	incRef         func()
	decRef         func() error // closes the listener on final decrement
	// err will return the server error if it encounters one before being closed gracefully
	// the value should be checked before using the reference
	// a non-nil value implies the connection is invalid and should be reconstructed
	// note that a nil value does not imply the listener nor server are, or will remain valid
	// as it can encounter an error or become closed at any time, asynchronously
	// (so expect listen related errors to be possible at points of use)
	err func() error
}
