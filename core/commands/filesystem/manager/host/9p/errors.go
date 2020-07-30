package p9fsp

import (
	"fmt"
	"syscall"

	fserrors "github.com/ipfs/go-ipfs/filesystem/errors"
)

func interpretError(err error) error {
	if errIntf, ok := err.(fserrors.Error); ok {
		// TODO: translate error values; placeholder for now; prints to console and cancels the request
		return map[fserrors.Kind]error{ // translation table for interface.Error  -> 9P2000.L error (Linux standard errno's)
			fserrors.Other:            err,
			fserrors.InvalidItem:      err,
			fserrors.InvalidOperation: err,
			fserrors.Permission:       err,
			fserrors.IO:               err,
			fserrors.Exist:            err,
			fserrors.NotExist:         syscall.Errno(0x02),
			fserrors.IsDir:            syscall.Errno(0x14),
			fserrors.NotDir:           err,
			fserrors.NotEmpty:         err,
		}[errIntf.Kind()]
	}

	panic(fmt.Sprintf("provided error is not translatable to 9P error %#v", err))
}
