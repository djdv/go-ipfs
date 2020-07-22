//+build !nofuse

package fuse

import (
	"fmt"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/ipfs/go-ipfs/filesystem/errors"
)

func interpretError(err error) errNo {
	if errIntf, ok := err.(errors.Error); ok {
		return map[errors.Kind]errNo{ // translation table for interface.Error -> FUSE error
			errors.Other:            -fuselib.EIO,
			errors.InvalidItem:      -fuselib.EINVAL,
			errors.InvalidOperation: -fuselib.ENOSYS,
			errors.Permission:       -fuselib.EACCES,
			errors.IO:               -fuselib.EIO,
			errors.Exist:            -fuselib.EEXIST,
			errors.NotExist:         -fuselib.ENOENT,
			errors.IsDir:            -fuselib.EISDIR,
			errors.NotDir:           -fuselib.ENOTDIR,
			errors.NotEmpty:         -fuselib.ENOTEMPTY,
		}[errIntf.Kind()]
	}

	panic(fmt.Sprintf("provided error is not translatable to POSIX error %#v", err))
}
