package mount9p

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	config "github.com/ipfs/go-ipfs-config"
	"github.com/multiformats/go-multiaddr"
)

var errObjectNotInitalized = errors.New("method called on uninitalized object")

const (
	tmplHome     = "IPFS_HOME"
	sun_path_len = 108
)

//TODO: var tmplRoot = `/` || ${CurDrive}:\ || ...

func stabilizeUnixPath(maString string) (string, error) {
	templateValueRepoPath, err := config.PathRoot()
	if err != nil {
		return "", err
	}

	if !filepath.IsAbs(templateValueRepoPath) { // stabilize root path
		absRepo, err := filepath.Abs(templateValueRepoPath)
		if err != nil {
			return "", err
		}
		templateValueRepoPath = absRepo
	}

	// expand templates

	// prevent template from expanding to double slashed paths like `/unix//home/...` on *nix systems
	// but allow it to expand to `/unix/C:\Users...` on the Windows platform
	templateValueRepoPath = strings.TrimPrefix(templateValueRepoPath, "/")

	// only expand documented template keys, not everything
	return os.Expand(maString, func(key string) string {
		return (map[string]string{
			tmplHome: templateValueRepoPath,
		})[key]
	}), nil
}

// TODO: multiaddr encapsulation conserns; this is just going to destroy every socket, not just ours
// it should probably just operate on the final component
func removeUnixSockets(maddr multiaddr.Multiaddr) error {
	var retErr error

	multiaddr.ForEach(maddr, func(comp multiaddr.Component) bool {
		if comp.Protocol().Code == multiaddr.P_UNIX {
			target := comp.Value()
			if runtime.GOOS == "windows" {
				target = strings.TrimLeft(target, "/")
			}
			if len(target) >= sun_path_len {
				// TODO [anyone] this type of check is platform dependant and checks+errors around it should exist in `mulitaddr` when forming the actual structure
				// e.g. on Windows 1909 and lower, this will always fail when binding
				// on Linux this can cause problems if applications are not aware of the true addr length and assume `sizeof addr <= 108`

				// FIXME: we lost our logger in the port from plugin; this shouldn't use fmt
				// logger.Warning("Unix domain socket path is at or exceeds standard length `sun_path[108]` this is likely to cause problems")
				fmt.Printf("[WARNING] Unix domain socket path %q is at or exceeds standard length `sun_path[108]` this is likely to cause problems\n", target)
			}

			// discard notexist errors
			if callErr := os.Remove(target); callErr != nil && !os.IsNotExist(callErr) {
				retErr = callErr
				return false // break out of ForEach
			}
		}
		return true // continue
	})

	return retErr
}
