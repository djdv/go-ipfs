package ufs

import (
	"errors"

	"github.com/ipfs/go-ipfs/filesystem"
	fserrors "github.com/ipfs/go-ipfs/filesystem/errors"
	interfaceutils "github.com/ipfs/go-ipfs/filesystem/interface"
)

var errNotImplemented = &interfaceutils.Error{
	Cause: errors.New("operation not supported by this node"),
	Type:  fserrors.InvalidOperation,
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
