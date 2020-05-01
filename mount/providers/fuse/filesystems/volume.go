package fusecommon

import (
	"errors"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
)

var Statfs func(string, *fuselib.Statfs_t) (error, int) = statfsUnsupported

func statfsUnsupported(_ string, _ *fuselib.Statfs_t) (error, int) {
	return errors.New("not implemented on this platform"), -fuselib.ENOSYS
}
