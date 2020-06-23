package mfs

import (
	"context"

	"github.com/ipfs/go-ipfs/filesystem"
	interfaceutils "github.com/ipfs/go-ipfs/filesystem/interfaces"
	gomfs "github.com/ipfs/go-mfs"
)

// adapts the MFS Root to our filesystem interface
type mfsInterface struct {
	ctx   context.Context
	mroot *gomfs.Root
}

var _ filesystem.Interface = (*mfsInterface)(nil)

func NewInterface(ctx context.Context, mroot *gomfs.Root) *mfsInterface {
	return &mfsInterface{
		ctx:   ctx,
		mroot: mroot,
	}
}

func (mi *mfsInterface) Close() error {
	return mi.mroot.Close()
}

func (mi *mfsInterface) Rename(oldName, newName string) error {
	if err := gomfs.Mv(mi.mroot, oldName, newName); err != nil {
		return &interfaceutils.Error{Cause: err, Type: filesystem.ErrorIO}
	}
	return nil
}
