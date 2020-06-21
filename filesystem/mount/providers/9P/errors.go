package p9fsp

import (
	"errors"
	"fmt"

	"github.com/ipfs/go-ipfs/filesystem"
)

func interpretError(err error) error {
	return err // TODO: translate error values; placeholder for now

	if errIntf, ok := err.(filesystem.Error); ok {
		return kindTo9P[errIntf.Kind()]
	}

	panic(fmt.Sprintf("provided error is not translatable to 9P error %#v", err))
}

// TODO: we need to move the error Kind type out of filesystem
// so that it becomes fserror.Kind
// we don't want fs.ErrorKind either, but we may want to keep fs.Error rather than fserror.Error
var kindTo9P = map[filesystem.Kind]error{
	filesystem.ErrorOther:            errors.New("TODO"),
	filesystem.ErrorInvalidItem:      errors.New("TODO"),
	filesystem.ErrorInvalidOperation: errors.New("TODO"),
	filesystem.ErrorPermission:       errors.New("TODO"),
	filesystem.ErrorIO:               errors.New("TODO"),
	filesystem.ErrorExist:            errors.New("TODO"),
	filesystem.ErrorNotExist:         errors.New("TODO"),
	filesystem.ErrorIsDir:            errors.New("TODO"),
	filesystem.ErrorNotDir:           errors.New("TODO"),
	filesystem.ErrorNotEmpty:         errors.New("TODO"),
}
