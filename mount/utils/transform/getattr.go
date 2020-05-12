package transform

import (
	"context"

	chunk "github.com/ipfs/go-ipfs-chunker"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-unixfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

// TODO: [investigate] [b6150f2f-8689-4e60-a605-fd40c826c32d]
// GetAttr resolves an IPFS API path and returns the attr, filled attr members, and error associated with the path
func GetAttr(ctx context.Context, path corepath.Path, core coreiface.CoreAPI, req IPFSStatRequest) (*IPFSStat, IPFSStatRequest, error) {
	ipldNode, err := core.ResolveNode(ctx, path)
	if err != nil {
		return nil, IPFSStatRequest{}, err
	}

	return ipldAttr(ctx, ipldNode, req)
}

// returns attr, filled members, error
func ipldAttr(ctx context.Context, node ipld.Node, req IPFSStatRequest) (*IPFSStat, IPFSStatRequest, error) {
	var (
		attr        IPFSStat
		filledAttrs IPFSStatRequest
	)
	ufsNode, err := unixfs.ExtractFSNode(node)
	if err != nil {
		return nil, filledAttrs, err
	}

	if req.Type {
		attr.FileType, filledAttrs.Type = unixfsTypeToCoreType(ufsNode.Type()), true
	}

	if req.Blocks {
		// TODO: when/if UFS supports this metadata field, use it instead
		attr.BlockSize, filledAttrs.Blocks = uint64(chunk.DefaultBlockSize), true
	}

	if req.Size {
		attr.Size, filledAttrs.Size = ufsNode.FileSize(), true
	}

	// TODO [eventually]: handle time metadata in new UFS format standard

	return &attr, filledAttrs, nil
}
