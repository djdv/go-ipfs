package mfs

import (
	"context"
	"errors"
	"fmt"
	gopath "path"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	gomfs "github.com/ipfs/go-mfs"
	"github.com/ipfs/go-unixfs"
	unixpb "github.com/ipfs/go-unixfs/pb"
)

func Unlink(mroot *gomfs.Root, path string) transform.Error { return remove(mroot, path, gomfs.TFile) }
func Rmdir(mroot *gomfs.Root, path string) transform.Error  { return remove(mroot, path, gomfs.TDir) }

// TODO: needs 9P errors
func remove(mroot *gomfs.Root, path string, nodeType gomfs.NodeType) transform.Error {
	// prepare to separate child from parent
	parentDir, childName, tErr := divorce(mroot, path)
	if tErr != nil {
		return tErr
	}

	childNode, err := parentDir.Child(childName)
	if err != nil {
		return &transform.ErrorActual{GoErr: err, ErrNo: -fuselib.ENOENT}
	}

	// check behavior for specific types
	switch nodeType {
	case gomfs.TFile:
		if !gomfs.IsFile(childNode) {

			// make sure it's not a (UFS) symlink
			ipldNode, err := childNode.GetNode()
			if err != nil {
				return &transform.ErrorActual{GoErr: err, ErrNo: -fuselib.EPERM}
			}
			ufsNode, err := unixfs.ExtractFSNode(ipldNode)
			if err != nil {
				return &transform.ErrorActual{GoErr: err, ErrNo: -fuselib.EPERM}
			}
			if t := ufsNode.Type(); t != unixpb.Data_Symlink {
				err := fmt.Errorf("%q is not a file or symlink (%q)", path, t)
				return &transform.ErrorActual{GoErr: err, ErrNo: -fuselib.EPERM}
			}
		}

	case gomfs.TDir:
		childDir, ok := childNode.(*gomfs.Directory)
		if !ok {
			err := fmt.Errorf("%q is not a directory (%T)", path, childNode)
			return &transform.ErrorActual{GoErr: err, ErrNo: -fuselib.ENOTDIR}
		}

		ents, err := childDir.ListNames(context.TODO())
		if err != nil {
			return &transform.ErrorActual{GoErr: err, ErrNo: -fuselib.EACCES}
		}

		if len(ents) != 0 {
			err := fmt.Errorf("directory %q is not empty", path)
			return &transform.ErrorActual{GoErr: err, ErrNo: -fuselib.ENOTEMPTY}
		}

	default:
		err := fmt.Errorf("unexpected node type %v", nodeType)
		return &transform.ErrorActual{GoErr: err, ErrNo: -fuselib.EACCES}
	}

	// unlink parent and child actually
	if err := parentDir.Unlink(childName); err != nil {
		return &transform.ErrorActual{GoErr: err, ErrNo: -fuselib.EACCES}
	}
	if err := parentDir.Flush(); err != nil {
		return &transform.ErrorActual{GoErr: err, ErrNo: -fuselib.EACCES}
	}

	return nil
}

func divorce(mroot *gomfs.Root, path string) (*gomfs.Directory, string, transform.Error) {
	parentPath, childName := gopath.Split(path)
	parentNode, err := gomfs.Lookup(mroot, parentPath)
	if err != nil {
		return nil, "", &transform.ErrorActual{GoErr: err, ErrNo: -fuselib.ENOENT}
	}

	parentDir, ok := parentNode.(*gomfs.Directory)
	if !ok {
		return nil, "", &transform.ErrorActual{GoErr: errors.New("parent isn't a directory"), ErrNo: -fuselib.ENOTDIR}
	}

	return parentDir, childName, nil
}
