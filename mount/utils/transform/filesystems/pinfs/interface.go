package pinfs

import (
	"context"

	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/ipfscore"
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
