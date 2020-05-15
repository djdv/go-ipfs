package mfs

import (
	"fmt"
	"os"
	gopath "path"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	dag "github.com/ipfs/go-merkledag"
	gomfs "github.com/ipfs/go-mfs"
	"github.com/ipfs/go-unixfs"
)

func Mknod(mroot *gomfs.Root, path string) transform.Error {
	parentPath, childName := gopath.Split(path)
	parentNode, err := gomfs.Lookup(mroot, parentPath)
	if err != nil {
		return &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.ENOENT,
			P9pErr: err,
		}
	}

	parentDir, ok := parentNode.(*gomfs.Directory)
	if !ok {
		return &transform.ErrorActual{
			GoErr:  fmt.Errorf("%s is not a directory", parentPath),
			ErrNo:  -fuselib.ENOTDIR,
			P9pErr: err,
		}
	}

	if _, err := parentDir.Child(childName); err == nil {
		return &transform.ErrorActual{
			GoErr:  fmt.Errorf("%q already exists", path),
			ErrNo:  -fuselib.EEXIST,
			P9pErr: err,
		}
	}

	dagFile := dag.NodeWithData(unixfs.FilePBData(nil, 0))
	dagFile.SetCidBuilder(parentDir.GetCidBuilder())
	if err := parentDir.AddChild(childName, dagFile); err != nil {
		return &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.EIO,
			P9pErr: err,
		}
	}

	return nil
}

func Mkdir(mroot *gomfs.Root, path string) transform.Error {
	if err := gomfs.Mkdir(mroot, path, gomfs.MkdirOpts{Flush: true}); err != nil {
		retErr := &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.EACCES,
			P9pErr: err,
		}
		if err == os.ErrExist {
			retErr.ErrNo = -fuselib.EEXIST
		}
		return retErr
	}
	return nil
}

func Symlink(mroot *gomfs.Root, path string, linkTarget string) transform.Error {
	parentPath, linkName := gopath.Split(path)

	parentNode, err := gomfs.Lookup(mroot, parentPath)
	if err != nil {
		return &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.ENOENT,
			P9pErr: err,
		}
	}

	parentDir, ok := parentNode.(*gomfs.Directory)
	if !ok {
		return &transform.ErrorActual{
			GoErr:  fmt.Errorf("%s is not a directory", parentPath),
			ErrNo:  -fuselib.ENOTDIR,
			P9pErr: err,
		}
	}

	if _, err := parentDir.Child(linkName); err == nil {
		return &transform.ErrorActual{
			GoErr:  fmt.Errorf("%q already exists", path),
			ErrNo:  -fuselib.EEXIST,
			P9pErr: err,
		}
	}

	dagData, err := unixfs.SymlinkData(linkTarget)
	if err != nil {
		return &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.ENOENT,
			P9pErr: err,
		}
	}

	// TODO: same note as on keyfs; use raw node's for this if we can
	dagNode := dag.NodeWithData(dagData)
	dagNode.SetCidBuilder(parentDir.GetCidBuilder())

	if err := parentDir.AddChild(linkName, dagNode); err != nil {
		return &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.ENOENT,
			P9pErr: err,
		}
	}
	return nil
}
