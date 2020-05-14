package transform

import (
	"context"
	"fmt"

	chunk "github.com/ipfs/go-ipfs-chunker"
	dag "github.com/ipfs/go-merkledag"
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

	switch typedNode := ipldNode.(type) {
	case *dag.ProtoNode:
		ufsNode, err := unixfs.ExtractFSNode(typedNode)
		if err != nil {
			return nil, IPFSStatRequest{}, err
		}
		return unixFSAttr(ctx, ufsNode, req)
	case *dag.RawNode:
		return rawAttr(ctx, typedNode, req)
	default:
		return nil, IPFSStatRequest{}, fmt.Errorf("unexpected node type: %T", typedNode)
	}
}

func rawAttr(ctx context.Context, rawNode *dag.RawNode, req IPFSStatRequest) (*IPFSStat, IPFSStatRequest, error) {
	var (
		attr        IPFSStat
		filledAttrs IPFSStatRequest
	)

	if req.Type {
		// raw nodes only contain data so we'll treat them as a flat file
		attr.FileType, filledAttrs.Type = coreiface.TFile, true
	}

	if req.Blocks {
		nodeStat, err := rawNode.Stat()
		if err != nil {
			return &attr, filledAttrs, err
		}
		attr.BlockSize, filledAttrs.Blocks = uint64(nodeStat.BlockSize), true
	}

	if req.Size {
		size, err := rawNode.Size()
		if err != nil {
			return &attr, filledAttrs, err
		}
		attr.Size, filledAttrs.Size = size, true
	}

	return &attr, filledAttrs, nil
}

// returns attr, filled members, error
func unixFSAttr(ctx context.Context, ufsNode *unixfs.FSNode, req IPFSStatRequest) (*IPFSStat, IPFSStatRequest, error) {
	var (
		attr        IPFSStat
		filledAttrs IPFSStatRequest
	)

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
