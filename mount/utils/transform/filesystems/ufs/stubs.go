package ufs

import (
	"errors"

	"github.com/ipfs/go-ipfs/mount/utils/transform"
	transcom "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems"
)

var errNotImplemented = &transcom.Error{
	Cause: errors.New("operation not supported by this system"),
	Type:  transform.ErrorInvalidOperation,
}

// TODO: we can implement directories, but currently have no use for them here
func (*ufsInterface) OpenDirectory(_ string) (transform.Directory, error) {
	return nil, errNotImplemented
}
func (*ufsInterface) Make(_ string) error            { return errNotImplemented }
func (*ufsInterface) MakeDirectory(_ string) error   { return errNotImplemented }
func (*ufsInterface) MakeLink(_, _ string) error     { return errNotImplemented }
func (*ufsInterface) Remove(_ string) error          { return errNotImplemented }
func (*ufsInterface) RemoveDirectory(_ string) error { return errNotImplemented }
func (*ufsInterface) RemoveLink(_ string) error      { return errNotImplemented }
func (*ufsInterface) Rename(_, _ string) error       { return errNotImplemented }
