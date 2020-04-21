package ipns

import (
	"context"

	"github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/ipfs"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

// TODO: figure out a nice way to just have the IPFS object prefixed with `/ipns` for its paths
var log = logging.Logger("fuse/ipns")

type FileSystem struct {
	*ipfs.FileSystem
}

func NewFileSystem(ctx context.Context, core coreiface.CoreAPI, opts ...ipfs.Option) *FileSystem {
	return &FileSystem{
		FileSystem: ipfs.NewFileSystem(ctx, core, opts...),
	}
}
