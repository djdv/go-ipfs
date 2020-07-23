package host

import (
	"fmt"
	"io"
)

// TODO: enum iota/stringer type
// and move to a another package
// fspath.Parse(string); fspath.HasSocket(...), IsFuse...
const (
	PathNamespace   = "host"
	SocketNamespace = "socket"
)

type (
	/*
		Request struct {
			Target    string   // the argument for the host's target parameter; e.g. host file system path
			Arguments []string // additional arguments (if any); e.g. fuse args, 9P socket address, etc.
		}
	*/

	Request interface {
		fmt.Stringer // this request's target, printed as a path
		// e.g. a file system path `/host/mnt/x`
		// a system socket `/socket/ip4/127.0.0.1/tcp/564`
		// etc.
		Arguments() []string // additional arguments (if any)
		// e.g. libfuse args, 9P server multiaddr
		// etc.
	}

	// Binding is a requests that has been bound to a target on the host
	// the Closer undoes the binding
	Binding struct {
		Request   // the request that initiated this binding
		io.Closer // decouples the target from the host
	}

	Response = struct {
		Error error
		Binding
	}
)

// Attacher dispatches requests to the host system.
// Translating the manager HostRequest's into host specific requests,
// Returning a stream of the operation's result
// (NOTE: channel should always be read until closed)
/* TODO: [lint] every host attacher should have unique structured requests that it creates and receives
we can't instantiate instances without parsing the request again, which should be done (once) in the parser package
not the constructor
type Attacher interface {
	Attach(...Request) <-chan Response // Attach couples instance implementations to the request's target
	//Detach(...HostRequest) <-chan Response // Detach removes a previously bound request
	//	List() <-chan Response             // List provides streams of prior (processed) instances
	//	io.Closer                          // closes all active binds
}
*/
