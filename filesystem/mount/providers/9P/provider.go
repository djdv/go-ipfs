package p9fsp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	ninelib "github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-ipfs/filesystem/mount"
	provcom "github.com/ipfs/go-ipfs/filesystem/mount/providers"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
)

// the file system instance provider itself
type p9pProvider struct {
	sync.Mutex
	log logging.EventLogger
	// TODO: this concept still needs to be discussed
	// it's here just for plumbing; when it becomes real the fixtures will already be in place
	resLock provcom.ResourceLock

	// FS provider
	ctx       context.Context // TODO: `Close` when canceled
	cancel    context.CancelFunc
	srv       *ninelib.Server            // the actual system we provide, as a network service
	instances provcom.InstanceCollection // active instances we've provided

	// 9P transport(s)
	servers map[string]serverRef
}

func NewProvider(ctx context.Context, namespace mount.Namespace, core coreiface.CoreAPI, opts ...provcom.Option) (mount.Provider, error) {
	opts = provcom.MaybeAppendLog(opts, LogGroup)
	settings := provcom.ParseOptions(opts...)

	// construct the system we're expected to provide
	var attacher ninelib.Attacher
	switch namespace {
	case mount.NamespaceIPFS, mount.NamespaceIPNS:
		attacher = CoreAttacher(ctx, core, namespace)
	case mount.NamespacePinFS:
		attacher = PinAttacher(ctx, core)
	case mount.NamespaceKeyFS:
		attacher = KeyAttacher(ctx, core)
	case mount.NamespaceFiles:
		attacher = MutableAttacher(ctx, settings.FilesAPIRoot)
	// TODO:
	//case mount.NamespaceCombined:
	default:
		return nil, fmt.Errorf("unknown namespace: %v", namespace)
	}

	provCtx, cancel := context.WithCancel(ctx)
	return &p9pProvider{
		log:       settings.Log,
		ctx:       provCtx,
		cancel:    cancel,
		srv:       ninelib.NewServer(attacher),
		resLock:   settings.ResourceLock,
		servers:   make(map[string]serverRef),
		instances: provcom.NewInstanceCollection(),
	}, nil
}

func (pr *p9pProvider) List() []mount.Request {
	pr.Lock()
	defer pr.Unlock()
	return pr.instances.List()
}

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
		host, port, err := net.SplitHostPort(addr.String())
		if err != nil {
			return err
		}
		mSource = host
		mArgs = "port=" + port
	default:
		return fmt.Errorf("%q is not a supported protocol", network)
	}

	return PlatformAttach(mSource, target, mArgs)
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
	// We don't have to care about a bool's value being fully written or not, but a partially written `error` is an interface with an arbitrary value
	// `decRef` has synchronization, so it may use `srvErr` directly (after synching)
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

// TODO: support targets that start with `maddr://` which just creates the socket and doesn't mount
// useful for systems that don't have fuse but do have plan9port, as well as allowing remote mounting
// e.g. starting the service on a TCP port would allow you to mount the instance on another machine
func (pr *p9pProvider) Bind(requests ...mount.Request) error {
	if len(requests) == 0 {
		return nil
	}

	pr.Lock()
	defer pr.Unlock()

	var (
		err           error
		instanceStack = provcom.NewInstanceStack(len(requests))
	)
	defer instanceStack.Clear()

	for _, req := range requests {
		var (
			instanceTarget string
			systemBind     bool
		)

		if req.Target != "" { // if the request target was provided, try to bind it to the host system
			instanceTarget = req.Target
			systemBind = true
		} else { // otherwise just spawn the listener exclusively
			instanceTarget = req.Parameter
		}

		if pr.instances.Exists(instanceTarget) {
			err = fmt.Errorf("%q already bound", instanceTarget)
			break
		}

		server, ok := pr.servers[req.Parameter]
		if !ok {
			server, err = listen(pr.ctx, req.Parameter, pr.srv)
			if err != nil {
				break
			}
			pr.servers[req.Parameter] = server
		}
		server.incRef()
		if !systemBind {
			instanceStack.Push(req, closer(server.decRef))
			requests = requests[1:] // shift successful requests out of the slice
			continue                // request was for a listener only, we're done
		}

		// otherwise, try to mount the target via a client connection to the server
		if err = mountListener(req.Target, server.Listener.Addr()); err != nil {
			if sErr := server.decRef(); sErr != nil {
				// TODO: if this is the only request for this server and the mount fails
				// the server will complain about the listener being close
				// by returning an accept error that was not filtered because
				// of a non-graceful stop
				// we need to filter that somehow here
				// statredServer bool above
				// if started && err = accept; filter
				err = fmt.Errorf("%w; additionally the server encountered an error on `Close`: %s", err, sErr)
			}
			break
		}
		requests = requests[1:] // shift successful requests out of the slice
	}

	// TODO: repetition with fuse provider
	// we might want to make a few small util functions in the provider package
	if err != nil {
		failedRequest := requests[0]
		err = fmt.Errorf("failed to bind %q{9P service: %q}<->%q: %w", failedRequest.Namespace, failedRequest.Parameter, failedRequest.Target, err)
		if instanceStack.Length() == 0 {
			pr.log.Error(err)
		} else {
			pr.log.Errorf("%s; attempting to detach previous targets", err)
			if uErr := instanceStack.Unwind(); uErr != nil {
				pr.log.Error(uErr)
				err = fmt.Errorf("%w; %s", err, uErr)
			}
		}
		return err
	}

	pr.instances.Add(instanceStack)
	return nil
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

func (pr *p9pProvider) Detach(requests ...mount.Request) error {
	pr.Lock()
	defer pr.Unlock()
	return pr.instances.Detach(requests...)
}

func (pr *p9pProvider) Close() error {
	pr.Lock()
	defer pr.Unlock()
	return pr.instances.Close()
}
