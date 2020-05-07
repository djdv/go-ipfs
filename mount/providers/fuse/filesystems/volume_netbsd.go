package fusecommon

import fuselib "github.com/billziss-gh/cgofuse/fuse"

func init() {
	Statfs = func(_ string, _ *fuselib.Statfs_t) (error, int) { return nil, -fuselib.ENOSYS }
}
