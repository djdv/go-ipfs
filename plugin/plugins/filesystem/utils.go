package filesystem

import (
	"os"

	"github.com/multiformats/go-multiaddr"
)

// removeUnixSockets attempts to remove all unix domain paths from a multiaddr
// does not stop on error, returns last encountered error, except "not exist" errors
func removeUnixSockets(ma multiaddr.Multiaddr) error {
	var retErr error
	multiaddr.ForEach(ma, func(comp multiaddr.Component) bool {
		if comp.Protocol().Code == multiaddr.P_UNIX {
			if err := os.Remove(comp.Value()); err != nil && !os.IsNotExist(err) {
				retErr = err
			}
		}
		return false
	})
	return retErr
}
