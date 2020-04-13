package transform

import (
	"context"

	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-mfs"
	"github.com/ipfs/go-unixfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

// returns attr, filled members, error
func CoreGetAttr(ctx context.Context, path corepath.Path, core coreiface.CoreAPI, req IPFSStatRequest) (*IPFSStat, IPFSStatRequest, error) {
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

	if req.Mode {
		attr.FileType, filledAttrs.Mode = unixfsTypeToCoreType(ufsNode.Type()), true
	}

	if req.Blocks {
		// TODO: when/if UFS supports this metadata field, use it instead
		attr.BlockSize, filledAttrs.Blocks = UFS1BlockSize, true
	}

	if req.Size {
		attr.Size, filledAttrs.Size = ufsNode.FileSize(), true
	}

	// TODO [eventually]: handle time metadata in new UFS format standard

	return &attr, filledAttrs, nil
}
