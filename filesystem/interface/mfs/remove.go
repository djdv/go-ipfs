package mfs

import (
	"context"
	"fmt"
	gopath "path"

	fserrors "github.com/ipfs/go-ipfs/filesystem/errors"
	interfaceutils "github.com/ipfs/go-ipfs/filesystem/interface"
	gomfs "github.com/ipfs/go-mfs"
	"github.com/ipfs/go-unixfs"
	unixpb "github.com/ipfs/go-unixfs/pb"
)

func (mi *mfsInterface) Remove(path string) error {
	return mi.remove(path, gomfs.TFile)
}

func (mi *mfsInterface) RemoveLink(path string) error {
	return mi.remove(path, gomfs.TFile) // TODO: this is a gross hack; change the parameter to be a core type and switch on it properly inside remove
}

func (mi *mfsInterface) RemoveDirectory(path string) error {
	return mi.remove(path, gomfs.TDir)
}

func (mi *mfsInterface) remove(path string, nodeType gomfs.NodeType) error {
	// prepare to separate child from parent
	parentDir, childName, err := splitParentChild(mi.mroot, path)
	if err != nil {
		return err
	}

	childNode, err := parentDir.Child(childName)
	if err != nil {
		return &interfaceutils.Error{Cause: err, Type: fserrors.NotExist}
	}

	// check behavior for specific types
	switch nodeType {
	case gomfs.TFile:
		if !gomfs.IsFile(childNode) {
			// make sure it's not a (UFS) symlink
			ipldNode, err := childNode.GetNode()
			if err != nil {
				return &interfaceutils.Error{Cause: err, Type: fserrors.Permission}
			}
			ufsNode, err := unixfs.ExtractFSNode(ipldNode)
			if err != nil {
				return &interfaceutils.Error{Cause: err, Type: fserrors.Permission}
			}
			if t := ufsNode.Type(); t != unixpb.Data_Symlink {
				return &interfaceutils.Error{
					Cause: fmt.Errorf("%q is not a file or symlink (%q)", path, t),
					Type:  fserrors.Permission,
				}
			}
		}

	case gomfs.TDir:
		childDir, ok := childNode.(*gomfs.Directory)
		if !ok {
			return &interfaceutils.Error{
				Cause: fmt.Errorf("%q is not a directory (%T)", path, childNode),
				Type:  fserrors.NotDir,
			}
		}

		ents, err := childDir.ListNames(context.TODO())
		if err != nil {
			return &interfaceutils.Error{Cause: err, Type: fserrors.Permission}
		}

		if len(ents) != 0 {
			return &interfaceutils.Error{
				Cause: fmt.Errorf("directory %q is not empty", path),
				Type:  fserrors.NotEmpty,
			}
		}

	default:
		return &interfaceutils.Error{
			Cause: fmt.Errorf("unexpected node type %v", nodeType),
			Type:  fserrors.Permission,
		}
	}

	// unlink parent and child actually
	if err := parentDir.Unlink(childName); err != nil {
		return &interfaceutils.Error{Cause: err, Type: fserrors.Permission}
	}
	if err := parentDir.Flush(); err != nil {
		return &interfaceutils.Error{Cause: err, Type: fserrors.Permission}
	}

	return nil
}

func splitParentChild(mroot *gomfs.Root, path string) (*gomfs.Directory, string, error) {
	parentPath, childName := gopath.Split(path)
	parentNode, err := gomfs.Lookup(mroot, parentPath)
	if err != nil {
		return nil, "", &interfaceutils.Error{Cause: err, Type: fserrors.NotExist}
	}

	parentDir, ok := parentNode.(*gomfs.Directory)
	if !ok {
		err = fmt.Errorf("parent %q isn't a directory", parentPath)
		return nil, "", &interfaceutils.Error{Cause: err, Type: fserrors.NotDir}
	}

	return parentDir, childName, nil
}
