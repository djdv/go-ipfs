package mfs

import (
	"context"
	"fmt"
	gopath "path"

	"github.com/ipfs/go-ipfs/filesystem"
	interfaceutils "github.com/ipfs/go-ipfs/filesystem/interfaces"
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
		return &interfaceutils.Error{Cause: err, Type: filesystem.ErrorNotExist}
	}

	// check behavior for specific types
	switch nodeType {
	case gomfs.TFile:
		if !gomfs.IsFile(childNode) {
			// make sure it's not a (UFS) symlink
			ipldNode, err := childNode.GetNode()
			if err != nil {
				return &interfaceutils.Error{Cause: err, Type: filesystem.ErrorPermission}
			}
			ufsNode, err := unixfs.ExtractFSNode(ipldNode)
			if err != nil {
				return &interfaceutils.Error{Cause: err, Type: filesystem.ErrorPermission}
			}
			if t := ufsNode.Type(); t != unixpb.Data_Symlink {
				return &interfaceutils.Error{
					Cause: fmt.Errorf("%q is not a file or symlink (%q)", path, t),
					Type:  filesystem.ErrorPermission,
				}
			}
		}

	case gomfs.TDir:
		childDir, ok := childNode.(*gomfs.Directory)
		if !ok {
			return &interfaceutils.Error{
				Cause: fmt.Errorf("%q is not a directory (%T)", path, childNode),
				Type:  filesystem.ErrorNotDir,
			}
		}

		ents, err := childDir.ListNames(context.TODO())
		if err != nil {
			return &interfaceutils.Error{Cause: err, Type: filesystem.ErrorPermission}
		}

		if len(ents) != 0 {
			return &interfaceutils.Error{
				Cause: fmt.Errorf("directory %q is not empty", path),
				Type:  filesystem.ErrorNotEmpty,
			}
		}

	default:
		return &interfaceutils.Error{
			Cause: fmt.Errorf("unexpected node type %v", nodeType),
			Type:  filesystem.ErrorPermission,
		}
	}

	// unlink parent and child actually
	if err := parentDir.Unlink(childName); err != nil {
		return &interfaceutils.Error{Cause: err, Type: filesystem.ErrorPermission}
	}
	if err := parentDir.Flush(); err != nil {
		return &interfaceutils.Error{Cause: err, Type: filesystem.ErrorPermission}
	}

	return nil
}

func splitParentChild(mroot *gomfs.Root, path string) (*gomfs.Directory, string, error) {
	parentPath, childName := gopath.Split(path)
	parentNode, err := gomfs.Lookup(mroot, parentPath)
	if err != nil {
		return nil, "", &interfaceutils.Error{Cause: err, Type: filesystem.ErrorNotExist}
	}

	parentDir, ok := parentNode.(*gomfs.Directory)
	if !ok {
		err = fmt.Errorf("parent %q isn't a directory", parentPath)
		return nil, "", &interfaceutils.Error{Cause: err, Type: filesystem.ErrorNotDir}
	}

	return parentDir, childName, nil
}
