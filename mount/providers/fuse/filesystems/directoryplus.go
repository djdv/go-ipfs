package fusecommon

import (
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
)

type (
	// directoryPlus is used in `FillDir` to handle FUSE's readdir plus feature
	// (via a type assertion of objects returned from `UpgradeDirectory`)
	directoryPlus struct {
		transform.Directory
		StatFunc
	}

	StatFunc func(name string) *fuselib.Stat_t
)

// UpgradeDirectory binds a Directory and a means to get attributes for its elements
// this should be used to transform directories into readdir plus capable directories
// before being sent to `FillDir`
func UpgradeDirectory(d transform.Directory, sf StatFunc) transform.Directory {
	return directoryPlus{Directory: d, StatFunc: sf}
}
