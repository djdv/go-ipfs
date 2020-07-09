//+build !nofuse

package fuse

import (
	fuselib "github.com/billziss-gh/cgofuse/fuse"
)

var statfs = statfsUnsupported

func statfsUnsupported(_ string, _ *fuselib.Statfs_t) (int, error) {
	return -fuselib.ENOSYS, nil
}
