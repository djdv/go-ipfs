package ipns

import (
	"context"

	"github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-ipfs/plugin/plugins/filesystem/filesystems/ipfs"
	"github.com/ipfs/go-ipfs/plugin/plugins/filesystem/meta"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var _ p9.File = (*File)(nil)
var _ meta.WalkRef = (*File)(nil)

// IPNS exposes the IPNS API over a p9.File interface
// Walk does not expect a namespace, only its path argument
// e.g. `ipfs.Walk([]string("Qm...", "subdir")` not `ipfs.Walk([]string("ipns", "Qm...", "subdir")`

type File = ipfs.File

func Attacher(ctx context.Context, core coreiface.CoreAPI, ops ...meta.AttachOption) p9.Attacher {
	options := meta.AttachOps(ops...)
	return &File{
		CoreBase:    meta.NewCoreBase("/ipns", core, ops...),
		OverlayBase: meta.OverlayBase{ParentCtx: ctx},
		Parent:      options.Parent,
	}
}
