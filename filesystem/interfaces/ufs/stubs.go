package ufs

import (
	"errors"

	"github.com/ipfs/go-ipfs/filesystem"
	interfaceutils "github.com/ipfs/go-ipfs/filesystem/interfaces"
)

var errNotImplemented = &interfaceutils.Error{
	Cause: errors.New("operation not supported by this system"),
	Type:  filesystem.ErrorInvalidOperation,
}

// TODO: we can implement directories, but currently have no use for them here
func (*ufsInterface) OpenDirectory(_ string) (filesystem.Directory, error) {
	return nil, errNotImplemented
}
func (*ufsInterface) Make(_ string) error            { return errNotImplemented }
func (*ufsInterface) MakeDirectory(_ string) error   { return errNotImplemented }
func (*ufsInterface) MakeLink(_, _ string) error     { return errNotImplemented }
func (*ufsInterface) Remove(_ string) error          { return errNotImplemented }
func (*ufsInterface) RemoveDirectory(_ string) error { return errNotImplemented }
func (*ufsInterface) RemoveLink(_ string) error      { return errNotImplemented }
func (*ufsInterface) Rename(_, _ string) error       { return errNotImplemented }
