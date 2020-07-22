package mfs

import (
	"fmt"
	"os"

	"github.com/ipfs/go-ipfs/filesystem/errors"

	"github.com/ipfs/go-ipfs/filesystem"
	interfaceutils "github.com/ipfs/go-ipfs/filesystem/interface"
	gomfs "github.com/ipfs/go-mfs"
	"github.com/ipfs/go-unixfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

func (mi *mfsInterface) Info(path string, req filesystem.StatRequest) (*filesystem.Stat, filesystem.StatRequest, error) {
	var (
		attr   = new(filesystem.Stat)
		filled filesystem.StatRequest
	)

	mfsNode, err := gomfs.Lookup(mi.mroot, path)
	if err != nil {
		return attr, filled, &interfaceutils.Error{
			Cause: err,
			Type:  errors.NotExist,
		}
	}

	ipldNode, err := mfsNode.GetNode()
	if err != nil {
		return nil, filled, &interfaceutils.Error{
			Cause: err,
			Type:  errors.Other,
		}
	}

	ufsNode, err := unixfs.ExtractFSNode(ipldNode)
	if err != nil {
		return attr, filled, &interfaceutils.Error{
			Cause: err,
			Type:  errors.Other,
		}
	}

	if req.Type {
		switch mfsNode.Type() {
		case gomfs.TFile:
			attr.Type, filled.Type = coreiface.TFile, true
		case gomfs.TDir:
			attr.Type, filled.Type = coreiface.TDirectory, true
		default:
			// symlinks are not natively supported by MFS / the Files API but we support them
			nodeType := ufsNode.Type()
			if nodeType == unixfs.TSymlink {
				attr.Type, filled.Type = coreiface.TSymlink, true
				break
			}

			return attr, filled, &interfaceutils.Error{
				Cause: fmt.Errorf("unexpected node type %d", nodeType),
				Type:  errors.Other,
			}
		}
	}

	if req.Size {
		attr.Size, filled.Size = ufsNode.FileSize(), true
	}

	if req.Blocks && !filled.Blocks {
		// NOTE: we can't account for variable block size so we use the size of the first block only (if any)
		blocks := len(ufsNode.BlockSizes())
		if blocks > 0 {
			attr.BlockSize = ufsNode.BlockSize(0)
			attr.Blocks = uint64(blocks)
		}

		// 0 is a valid value for these fields, especially for non-regular files
		// so set this to true regardless of if one was provided or not
		filled.Blocks = true
	}

	return attr, filled, nil
}

func (mi *mfsInterface) ExtractLink(path string) (string, error) {
	mfsNode, err := gomfs.Lookup(mi.mroot, path)
	if err != nil {
		rErr := &interfaceutils.Error{Cause: err}
		if err == os.ErrNotExist {
			rErr.Type = errors.NotExist
			return "", rErr
		}
		rErr.Type = errors.Permission
		return "", rErr
	}

	ipldNode, err := mfsNode.GetNode()
	if err != nil {
		return "", &interfaceutils.Error{Cause: err, Type: errors.IO}
	}

	ufsNode, err := unixfs.ExtractFSNode(ipldNode)
	if err != nil {
		return "", &interfaceutils.Error{Cause: err, Type: errors.IO}
	}
	if ufsNode.Type() != unixfs.TSymlink {
		err := fmt.Errorf("%q is not a link", path)
		return "", &interfaceutils.Error{Cause: err, Type: errors.InvalidItem}
	}

	return string(ufsNode.Data()), nil
}
