package mfs

import (
	"fmt"
	"os"
	gopath "path"

	"github.com/ipfs/go-ipfs/filesystem/errors"

	interfaceutils "github.com/ipfs/go-ipfs/filesystem/interface"
	dag "github.com/ipfs/go-merkledag"
	gomfs "github.com/ipfs/go-mfs"
	"github.com/ipfs/go-unixfs"
)

func (mi *mfsInterface) Make(path string) error {
	parentPath, childName := gopath.Split(path)
	parentNode, err := gomfs.Lookup(mi.mroot, parentPath)
	if err != nil {
		return &interfaceutils.Error{
			Cause: err,
			Type:  errors.NotExist,
		}
	}

	parentDir, ok := parentNode.(*gomfs.Directory)
	if !ok {
		return &interfaceutils.Error{
			Cause: fmt.Errorf("%s is not a directory", parentPath),
			Type:  errors.NotDir,
		}
	}

	if _, err := parentDir.Child(childName); err == nil {
		return &interfaceutils.Error{
			Cause: fmt.Errorf("%q already exists", path),
			Type:  errors.Exist,
		}
	}

	dagFile := dag.NodeWithData(unixfs.FilePBData(nil, 0))
	dagFile.SetCidBuilder(parentDir.GetCidBuilder())
	if err := parentDir.AddChild(childName, dagFile); err != nil {
		return &interfaceutils.Error{
			Cause: err,
			Type:  errors.IO,
		}
	}

	return nil
}

func (mi *mfsInterface) MakeDirectory(path string) error {
	if err := gomfs.Mkdir(mi.mroot, path, gomfs.MkdirOpts{Flush: true}); err != nil {
		retErr := &interfaceutils.Error{
			Cause: err,
			Type:  errors.Permission,
		}
		if err == os.ErrExist {
			retErr.Type = errors.Exist
		}
		return retErr
	}
	return nil
}

func (mi *mfsInterface) MakeLink(path, linkTarget string) error {
	parentPath, linkName := gopath.Split(path)

	parentNode, err := gomfs.Lookup(mi.mroot, parentPath)
	if err != nil {
		return &interfaceutils.Error{
			Cause: err,
			Type:  errors.NotExist,
		}
	}

	parentDir, ok := parentNode.(*gomfs.Directory)
	if !ok {
		return &interfaceutils.Error{
			Cause: fmt.Errorf("%s is not a directory", parentPath),
			Type:  errors.NotDir,
		}
	}

	if _, err := parentDir.Child(linkName); err == nil {
		return &interfaceutils.Error{
			Cause: fmt.Errorf("%q already exists", path),
			Type:  errors.Exist,
		}
	}

	dagData, err := unixfs.SymlinkData(linkTarget)
	if err != nil {
		return &interfaceutils.Error{
			Cause: err,
			Type:  errors.NotExist,
		}
	}

	// TODO: same note as on keyfs; use raw node's for this if we can
	dagNode := dag.NodeWithData(dagData)
	dagNode.SetCidBuilder(parentDir.GetCidBuilder())

	if err := parentDir.AddChild(linkName, dagNode); err != nil {
		return &interfaceutils.Error{
			Cause: err,
			Type:  errors.NotExist, // SUSv7 "...or write permission is denied on the parent directory of the directory to be created"
		}
	}
	return nil
}
