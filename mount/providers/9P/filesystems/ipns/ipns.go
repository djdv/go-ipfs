// Package ipns exposes the Inter-Planetary Name System API as a 9P compatible resource server
package ipns

import (
	"context"

	"github.com/hugelgupf/p9/p9"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	common "github.com/ipfs/go-ipfs/mount/providers/9P/filesystems"
	"github.com/ipfs/go-ipfs/mount/providers/9P/filesystems/ipfs"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/ipfscore"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var _ p9.File = (*File)(nil)
var _ common.WalkRef = (*File)(nil)

// NOTE: this package is going to go away
// we're going to do the same thing we did in FUSE which is to merge IPFS and IPNS
// into a single core pkg that takes in a namespace to operate on
// under a more unified interface

// File exposes the IPNS API over a p9.File interface
// Walk does not expect a namespace, only its path argument
// e.g. `ipns.Walk([]string("Qm...", "subdir")` not `ipns.Walk([]string("ipns", "Qm...", "subdir")`
type File = ipfs.File

func Attacher(ctx context.Context, core coreiface.CoreAPI, ops ...common.AttachOption) p9.Attacher {
	options := common.AttachOps(ops...)
	return &File{
		CoreBase:    common.NewCoreBase("/ipns", core, ops...),
		OverlayBase: common.OverlayBase{ParentCtx: ctx},
		Parent:      options.Parent,
		Intf:        ipfscore.NewInterface(ctx, core, mountinter.NamespaceIPNS),
	}
}
