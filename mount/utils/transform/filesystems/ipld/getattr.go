package ipld

import (
	"context"

	"github.com/ipfs/go-ipfs/mount/utils/transform"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-unixfs"
)

// TODO [2019.09.12; anyone]
// Start a discussion around block sizes
// should we use the de-facto standard of 4KiB or use our own of 256KiB?
// context: https://github.com/ipfs/go-ipfs/pull/6612/files#r322989041
const ufs1BlockSize = 256 << 1

// returns attr, filled members, error
func GetAttr(ctx context.Context, node ipld.Node, req transform.IPFSStatRequest) (*transform.IPFSStat, transform.IPFSStatRequest, error) {
	var (
		attr        transform.IPFSStat
		filledAttrs transform.IPFSStatRequest
	)
	ufsNode, err := unixfs.ExtractFSNode(node)
	if err != nil {
		return nil, filledAttrs, err
	}

	if req.Type {
		attr.FileType, filledAttrs.Type = transform.UnixfsTypeToCoreType(ufsNode.Type()), true
	}

	if req.Blocks {
		// TODO: when/if UFS supports this metadata field, use it instead
		attr.BlockSize, filledAttrs.Blocks = ufs1BlockSize, true
	}

	if req.Size {
		attr.Size, filledAttrs.Size = ufsNode.FileSize(), true
	}

	// TODO [eventually]: handle time metadata in new UFS format standard

	return &attr, filledAttrs, nil
}
