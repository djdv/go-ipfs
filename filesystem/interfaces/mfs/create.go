package mfs

import (
	"fmt"
	"os"
	gopath "path"

	transform "github.com/ipfs/go-ipfs/filesystem"
	transcom "github.com/ipfs/go-ipfs/filesystem/interfaces"
	dag "github.com/ipfs/go-merkledag"
	gomfs "github.com/ipfs/go-mfs"
	"github.com/ipfs/go-unixfs"
)

func (mi *mfsInterface) Make(path string) error {
	parentPath, childName := gopath.Split(path)
	parentNode, err := gomfs.Lookup(mi.mroot, parentPath)
	if err != nil {
		return &transcom.Error{
			Cause: err,
			Type:  transform.ErrorNotExist,
		}
	}

	parentDir, ok := parentNode.(*gomfs.Directory)
	if !ok {
		return &transcom.Error{
			Cause: fmt.Errorf("%s is not a directory", parentPath),
			Type:  transform.ErrorNotDir,
		}
	}

	if _, err := parentDir.Child(childName); err == nil {
		return &transcom.Error{
			Cause: fmt.Errorf("%q already exists", path),
			Type:  transform.ErrorExist,
		}
	}

	dagFile := dag.NodeWithData(unixfs.FilePBData(nil, 0))
	dagFile.SetCidBuilder(parentDir.GetCidBuilder())
	if err := parentDir.AddChild(childName, dagFile); err != nil {
		return &transcom.Error{
			Cause: err,
			Type:  transform.ErrorIO,
		}
	}

	return nil
}

func (mi *mfsInterface) MakeDirectory(path string) error {
	if err := gomfs.Mkdir(mi.mroot, path, gomfs.MkdirOpts{Flush: true}); err != nil {
		retErr := &transcom.Error{
			Cause: err,
			Type:  transform.ErrorPermission,
		}
		if err == os.ErrExist {
			retErr.Type = transform.ErrorExist
		}
		return retErr
	}
	return nil
}

func (mi *mfsInterface) MakeLink(path string, linkTarget string) error {
	parentPath, linkName := gopath.Split(path)

	parentNode, err := gomfs.Lookup(mi.mroot, parentPath)
	if err != nil {
		return &transcom.Error{
			Cause: err,
			Type:  transform.ErrorNotExist,
		}
	}

	parentDir, ok := parentNode.(*gomfs.Directory)
	if !ok {
		return &transcom.Error{
			Cause: fmt.Errorf("%s is not a directory", parentPath),
			Type:  transform.ErrorNotDir,
		}
	}

	if _, err := parentDir.Child(linkName); err == nil {
		return &transcom.Error{
			Cause: fmt.Errorf("%q already exists", path),
			Type:  transform.ErrorExist,
		}
	}

	dagData, err := unixfs.SymlinkData(linkTarget)
	if err != nil {
		return &transcom.Error{
			Cause: err,
			Type:  transform.ErrorNotExist,
		}
	}

	// TODO: same note as on keyfs; use raw node's for this if we can
	dagNode := dag.NodeWithData(dagData)
	dagNode.SetCidBuilder(parentDir.GetCidBuilder())

	if err := parentDir.AddChild(linkName, dagNode); err != nil {
		return &transcom.Error{
			Cause: err,
			Type:  transform.ErrorNotExist, // SUSv7 "...or write permission is denied on the parent directory of the directory to be created"
		}
	}
	return nil
}
