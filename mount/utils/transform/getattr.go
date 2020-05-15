package transform

import (
	"context"

	chunk "github.com/ipfs/go-ipfs-chunker"
	ipld "github.com/ipfs/go-ipld-format"
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
		return unixFSAttr(ufsNode, req)

	// pretend Go allows this:
	// case *dag.RawNode, *cbor.Node:
	// fallthrough
	default:
		return genericAttr(typedNode, req)
	}
}

func genericAttr(genericNode ipld.Node, req IPFSStatRequest) (*IPFSStat, IPFSStatRequest, error) {
	var (
		attr        IPFSStat
		filledAttrs IPFSStatRequest
	)

	if req.Type {
		// raw nodes only contain data so we'll treat them as a flat file
		// cbor nodes are not currently supported via UnixFS so we assume them to contain only data
		// TODO: review ^ is there some way we can implement this that won't blow up in the future?
		// (if unixfs supports cbor and directories are implemented to use them )
		attr.FileType, filledAttrs.Type = coreiface.TFile, true
	}

	if req.Blocks {
		nodeStat, err := genericNode.Stat()
		if err != nil {
			return &attr, filledAttrs, err
		}
		attr.BlockSize, filledAttrs.Blocks = uint64(nodeStat.BlockSize), true
	}

	if req.Size {
		size, err := genericNode.Size()
		if err != nil {
			return &attr, filledAttrs, err
		}
		attr.Size, filledAttrs.Size = size, true
	}

	return &attr, filledAttrs, nil
}

// returns attr, filled members, error
func unixFSAttr(ufsNode *unixfs.FSNode, req IPFSStatRequest) (*IPFSStat, IPFSStatRequest, error) {
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
