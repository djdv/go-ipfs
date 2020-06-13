package mfs

import (
	"context"

	"github.com/ipfs/go-ipfs/mount/utils/transform"
	transcom "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems"
	gomfs "github.com/ipfs/go-mfs"
)

// adapts the MFS Root to our filesystem interface
type mfsInterface struct {
	ctx   context.Context
	mroot *gomfs.Root
}

var _ transform.Interface = (*mfsInterface)(nil)

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
		return &transcom.Error{Cause: err, Type: transform.ErrorIO}
	}
	return nil
}
