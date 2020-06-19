package mfs

import (
	"context"
	"fmt"
	gopath "path"

	transform "github.com/ipfs/go-ipfs/filesystem"
	transcom "github.com/ipfs/go-ipfs/filesystem/interfaces"
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
		return &transcom.Error{Cause: err, Type: transform.ErrorNotExist}
	}

	// check behavior for specific types
	switch nodeType {
	case gomfs.TFile:
		if !gomfs.IsFile(childNode) {

			// make sure it's not a (UFS) symlink
			ipldNode, err := childNode.GetNode()
			if err != nil {
				return &transcom.Error{Cause: err, Type: transform.ErrorPermission}
			}
			ufsNode, err := unixfs.ExtractFSNode(ipldNode)
			if err != nil {
				return &transcom.Error{Cause: err, Type: transform.ErrorPermission}
			}
			if t := ufsNode.Type(); t != unixpb.Data_Symlink {
				return &transcom.Error{
					Cause: fmt.Errorf("%q is not a file or symlink (%q)", path, t),
					Type:  transform.ErrorPermission}
			}
		}

	case gomfs.TDir:
		childDir, ok := childNode.(*gomfs.Directory)
		if !ok {
			return &transcom.Error{
				Cause: fmt.Errorf("%q is not a directory (%T)", path, childNode),
				Type:  transform.ErrorNotDir}
		}

		ents, err := childDir.ListNames(context.TODO())
		if err != nil {
			return &transcom.Error{Cause: err, Type: transform.ErrorPermission}
		}

		if len(ents) != 0 {
			return &transcom.Error{
				Cause: fmt.Errorf("directory %q is not empty", path),
				Type:  transform.ErrorNotEmpty}
		}

	default:
		return &transcom.Error{
			Cause: fmt.Errorf("unexpected node type %v", nodeType),
			Type:  transform.ErrorPermission}
	}

	// unlink parent and child actually
	if err := parentDir.Unlink(childName); err != nil {
		return &transcom.Error{Cause: err, Type: transform.ErrorPermission}
	}
	if err := parentDir.Flush(); err != nil {
		return &transcom.Error{Cause: err, Type: transform.ErrorPermission}
	}

	return nil
}

func splitParentChild(mroot *gomfs.Root, path string) (*gomfs.Directory, string, error) {
	parentPath, childName := gopath.Split(path)
	parentNode, err := gomfs.Lookup(mroot, parentPath)
	if err != nil {
		return nil, "", &transcom.Error{Cause: err, Type: transform.ErrorNotExist}
	}

	parentDir, ok := parentNode.(*gomfs.Directory)
	if !ok {
		err = fmt.Errorf("parent %q isn't a directory", parentPath)
		return nil, "", &transcom.Error{Cause: err, Type: transform.ErrorNotDir}
	}

	return parentDir, childName, nil
}
