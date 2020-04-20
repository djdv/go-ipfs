package transform

import (
	"context"

	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-mfs"
	"github.com/ipfs/go-unixfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

// TODO [2019.09.12; anyone]
// Start a discussion around block sizes
// should we use the de-facto standard of 4KiB or use our own of 256KiB?
// context: https://github.com/ipfs/go-ipfs/pull/6612/files#r322989041
const ufs1BlockSize = 256 << 10

// GetAttrCore returns attr, filled members, error
func GetAttrCore(ctx context.Context, path corepath.Path, core coreiface.CoreAPI, req IPFSStatRequest) (*IPFSStat, IPFSStatRequest, error) {
	// translate from abstract path to CoreAPI resolved path
	resolvedPath, err := core.ResolvePath(ctx, path)
	if err != nil {
		return nil, IPFSStatRequest{}, err
	}

	ipldNode, err := core.Dag().Get(ctx, resolvedPath.Cid())
	if err != nil {
		return nil, IPFSStatRequest{}, err
	}

	return ipldStat(ctx, ipldNode, req)
}

func MfsGetAttr(ctx context.Context, mroot *mfs.Root, mfsPath string, req IPFSStatRequest) (*IPFSStat, IPFSStatRequest, error) {
	mfsNode, err := mfs.Lookup(mroot, mfsPath)
	if err != nil {
		return nil, IPFSStatRequest{}, err
	}

	ipldNode, err := mfsNode.GetNode()
	if err != nil {
		return nil, IPFSStatRequest{}, err
	}

	attr, filled, err := ipldStat(ctx, ipldNode, req)
	if err != nil {
		return nil, IPFSStatRequest{}, err
	}
	return attr, filled, err
}

// returns attr, filled members, error
func ipldStat(ctx context.Context, node ipld.Node, req IPFSStatRequest) (*IPFSStat, IPFSStatRequest, error) {
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
		attr.BlockSize, filledAttrs.Blocks = ufs1BlockSize, true
	}

	if req.Size {
		attr.Size, filledAttrs.Size = ufsNode.FileSize(), true
	}

	// TODO [eventually]: handle time metadata in new UFS format standard

	return &attr, filledAttrs, nil
}
