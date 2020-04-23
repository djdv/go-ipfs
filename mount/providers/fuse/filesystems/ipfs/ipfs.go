package ipfs

import (
	"context"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	ipfscore "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/core"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

func NewFileSystem(ctx context.Context, core coreiface.CoreAPI, opts ...ipfscore.Option) fuselib.FileSystemInterface {
	opts = append(opts,
		ipfscore.WithLog(logging.Logger("fuse/ipfs")),
		ipfscore.WithNamespace(mountinter.NamespaceIPFS))

	return ipfscore.NewFileSystem(ctx, core, opts...)
}
