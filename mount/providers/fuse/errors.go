package fuse

import (
	"fmt"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
)

func interpretError(err error) errNo {
	if errIntf, ok := err.(transform.Error); ok {
		return kindToFuse[errIntf.Kind()]
	}

	panic(fmt.Sprintf("provided error is not translatable to POSIX error %#v", err))
}

var kindToFuse = map[transform.Kind]errNo{
	transform.ErrorOther:            -fuselib.EIO,
	transform.ErrorInvalidItem:      -fuselib.EINVAL,
	transform.ErrorInvalidOperation: -fuselib.ENOSYS,
	transform.ErrorPermission:       -fuselib.EACCES,
	transform.ErrorIO:               -fuselib.EIO,
	transform.ErrorExist:            -fuselib.EEXIST,
	transform.ErrorNotExist:         -fuselib.ENOENT,
	transform.ErrorIsDir:            -fuselib.EISDIR,
	transform.ErrorNotDir:           -fuselib.ENOTDIR,
	transform.ErrorNotEmpty:         -fuselib.ENOTEMPTY,
}
