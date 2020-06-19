package fuse

import (
	fuselib "github.com/billziss-gh/cgofuse/fuse"
)

var statfs func(string, *fuselib.Statfs_t) (int, error) = statfsUnsupported

func statfsUnsupported(_ string, _ *fuselib.Statfs_t) (int, error) {
	return -fuselib.ENOSYS, nil
}
