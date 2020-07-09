package host

import (
	"io"
)

type (
	Request struct {
		Target    string   // the argument for the host's target parameter; e.g. host file system path
		Arguments []string // additional arguments (if any); e.g. fuse args, 9P socket address, etc.
	}

	// Binding is a requests that has been bound to a target on the host
	// the Closer undoes the binding
	Binding struct {
		Request   // the target this fs is attached to
		io.Closer // decouples the target and fs
	}

	Response = struct {
		Binding
		Error error
	}
)

// Attacher dispatches requests to the host system.
// Translating the manager Request's into host specific requests,
// Returning a stream of the operation's result
// (NOTE: channel should always be read until closed)
type Attacher interface {
	Attach(...Request) <-chan Response // Attach couples instance implementations to the request's target
	//Detach(...Request) <-chan Response // Detach removes a previously bound request
	//	List() <-chan Response             // List provides streams of prior (processed) instances
	//	io.Closer                          // closes all active binds
}
