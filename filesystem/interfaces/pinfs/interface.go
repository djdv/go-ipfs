package pinfs

import (
	"context"

	transform "github.com/ipfs/go-ipfs/filesystem"
	"github.com/ipfs/go-ipfs/filesystem/interfaces/ipfscore"
	mountinter "github.com/ipfs/go-ipfs/filesystem/mount"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

type pinInterface struct {
	ctx  context.Context
	core coreiface.CoreAPI
	ipfs transform.Interface
}

func NewInterface(ctx context.Context, core coreiface.CoreAPI) transform.Interface {
	return &pinInterface{
		ctx:  ctx,
		core: core,
		ipfs: ipfscore.NewInterface(ctx, core, mountinter.NamespaceIPFS),
	}
}

func (pi *pinInterface) Close() error { return pi.ipfs.Close() }
func (pi *pinInterface) Rename(oldName, newName string) error {
	return pi.ipfs.Rename(oldName, newName)
}
