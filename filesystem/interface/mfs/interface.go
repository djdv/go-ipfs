package mfs

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/ipfs/go-ipfs/filesystem"
	fserrors "github.com/ipfs/go-ipfs/filesystem/errors"
	interfaceutils "github.com/ipfs/go-ipfs/filesystem/interface"
	gomfs "github.com/ipfs/go-mfs"
)

// adapts the MFS Root to our filesystem node
type mfsInterface struct {
	ctx   context.Context
	mroot *gomfs.Root
}

func NewInterface(ctx context.Context, mroot *gomfs.Root) (fs filesystem.Interface, err error) {
	if mroot == nil {
		err = fmt.Errorf("MFS root was not provided")
		return
	}

	fs = &mfsInterface{
		ctx:   ctx,
		mroot: mroot,
	}
	return
}

func (mi *mfsInterface) ID() filesystem.ID { return filesystem.Files } // TODO: distinct ID
func (mi *mfsInterface) Close() error      { return mi.mroot.Close() }
func (mi *mfsInterface) Rename(oldName, newName string) error {
	if err := gomfs.Mv(mi.mroot, oldName, newName); err != nil {
		return &interfaceutils.Error{Cause: err, Type: fserrors.IO}
	}
	return nil
}

func mfsLookupErr(path string, err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return interfaceutils.ErrNotExist(path)
	}
	return &interfaceutils.Error{
		Cause: fmt.Errorf("%w: %s", err, path),
		Type:  fserrors.Other,
	}
}
