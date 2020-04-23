package mfs

import (
	"context"

	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/ipfscore"
	"github.com/ipfs/go-mfs"
)

0

func MfsGetAttr(ctx context.Context, mroot *mfs.Root, mfsPath string, req IPFSStatRequest) (*IPFSStat, IPFSStatRequest, error) {
	mfsNode, err := mfs.Lookup(mroot, mfsPath)
	if err != nil {
		return nil, IPFSStatRequest{}, err
	}

	ipldNode, err := mfsNode.GetNode()
	if err != nil {
		return nil, IPFSStatRequest{}, err
	}

	attr, filled, err := ipfscore.GetAttrIPLD(ctx, ipldNode, req)
	if err != nil {
		return nil, IPFSStatRequest{}, err
	}
	return attr, filled, err
}
