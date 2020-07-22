package p9fsp

import (
	"fmt"

	"github.com/ipfs/go-ipfs/filesystem/errors"
)

func interpretError(err error) error {
	if errIntf, ok := err.(errors.Error); ok {
		// TODO: translate error values; placeholder for now; prints to console and cancels the request
		return map[errors.Kind]error{ // translation table for interface.Error  -> 9P error
			errors.Other:            err,
			errors.InvalidItem:      err,
			errors.InvalidOperation: err,
			errors.Permission:       err,
			errors.IO:               err,
			errors.Exist:            err,
			errors.NotExist:         err,
			errors.IsDir:            err,
			errors.NotDir:           err,
			errors.NotEmpty:         err,
		}[errIntf.Kind()]
	}

	panic(fmt.Sprintf("provided error is not translatable to 9P error %#v", err))
}
