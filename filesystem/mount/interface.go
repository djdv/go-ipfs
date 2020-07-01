package mount

import (
	"io"
	"strings"
)

/* TODO [current]
change filesystem/mount to filesystem/manager
/filesystem/mount/providers to filesystem/manager/interfaces/

providers must re-use existing namespaces
e.g. if IPFS is mounted to 2 locations via fuse
the fuse provider should have a single instance of ipfs.FileSystem

Fuse parameters should be in the target parameter string, not tied to the provider itself
(move fuseargs into the parser when provider == fuse, like is done for 9P)

params should be shown in `ipfs mount -l` but only when `-v` is provided

list output should look like this
Fuse:
	IPFS
		/somewhere [-params -if -verbose]
		/somewhereElse
	IPNS:
		/somewhereDifferent
...
*/

const LogGroup = "filesystem"

// Request specifies parameters for a system implementation.
type Request struct {
	Namespace        // the system you're requesting to interact with,
	Target    string // the target you wish to couple it with,
	Parameter string // and the system specific parameters (if any)
}

// Interface dispatches `Request`s to specific `Provider`s.
// e.g. it is a `Provider` multiplexer.
// TODO: [bb846ad6-69aa-4f5c-991c-626a7ce92b38] name considerations
// manager.Interface vs stutter manager.Manager
type Interface interface {
	// proxies methods to providers
	Bind(ProviderType, ...Request) error
	Detach(ProviderType, ...Request) error
	// List provides the mapping of active `Provider`s and their instantiated `Request`s
	List() map[ProviderType][]Request
	io.Closer // closes all active providers
}

// Provider contains the methods to provide the `Interface` with `Instance`s of a system.
// Binding their implementation, to a `Request`'s target.
type Provider interface {
	// Bind couples request targets with their system implementation
	Bind(...Request) error
	// List returns a slice of `Request`s that are currently instantiated
	List() []Request
	// Detach closes specific bindings
	Detach(...Request) error
	io.Closer // closes all active binds
}

// TODO: remove this; localize the string method wherever it's printed
type targetCollections []Request

func (pairs targetCollections) String() string {
	var prettyPaths strings.Builder
	tEnd := len(pairs) - 1
	for i, pair := range pairs {
		prettyPaths.WriteRune('"')
		prettyPaths.WriteString(pair.Target)
		prettyPaths.WriteRune('"')
		if i != tEnd {
			prettyPaths.WriteString(", ")
		}
	}
	return prettyPaths.String()
}
