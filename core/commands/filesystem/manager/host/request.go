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
	Request interface {
		fmt.Stringer // this request's target, printed as a path
		// e.g. a file system path `/host/mnt/x`
		// a system socket `/socket/ip4/127.0.0.1/tcp/564`
		// etc.
		Arguments() []string // additional arguments (if any)
		// e.g. libfuse args, 9P server multiaddr
		// etc.
	}

	// Binding is a requests that has been bound to a target on the host,
	// its Closer undoes the binding.
	Binding struct {
		Request   // the request that initiated this binding
		io.Closer // decouples the target from the host
	}

	Response = struct {
		Error error
		Binding
	}
)
