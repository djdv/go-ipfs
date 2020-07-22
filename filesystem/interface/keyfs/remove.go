package keyfs

import (
	"fmt"

	"github.com/ipfs/go-ipfs/filesystem/errors"

	"github.com/ipfs/go-ipfs/filesystem"
	interfaceutils "github.com/ipfs/go-ipfs/filesystem/interface"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

func (ki *keyInterface) Remove(path string) error {
	fs, _, fsPath, deferFunc, err := ki.selectFS(path)
	if err != nil {
		return err
	}
	defer deferFunc()

	if fs == ki {
		return ki.remove(fsPath, coreiface.TFile)
	}
	return fs.Remove(fsPath)
}

func (ki *keyInterface) RemoveLink(path string) error {
	fs, _, fsPath, deferFunc, err := ki.selectFS(path)
	if err != nil {
		return err
	}
	defer deferFunc()

	if fs == ki {
		return ki.remove(fsPath, coreiface.TSymlink)
	}
	return fs.RemoveLink(fsPath)
}

func (ki *keyInterface) RemoveDirectory(path string) error {
	fs, _, fsPath, deferFunc, err := ki.selectFS(path)
	if err != nil {
		return err
	}
	defer deferFunc()

	if fs == ki {
		return ki.remove(fsPath, coreiface.TDirectory)
	}
	return fs.RemoveDirectory(fsPath)
}

func (ki *keyInterface) remove(path string, nodeType coreiface.FileType) error {
	iStat, _, err := ki.Info(path, filesystem.StatRequest{Type: true})
	if err != nil {
		return err
	}

	if iStat.Type != nodeType {
		switch nodeType {
		case coreiface.TFile:
			return &interfaceutils.Error{
				Cause: fmt.Errorf("%q is not a file", path),
				Type:  errors.IsDir,
			}
		case coreiface.TDirectory:
			return &interfaceutils.Error{
				Cause: fmt.Errorf("%q is not a directory", path),
				Type:  errors.NotDir,
			}
		case coreiface.TSymlink:
			return &interfaceutils.Error{
				Cause: fmt.Errorf("%q is not a link", path),
				Type:  errors.NotExist, // TODO: [review] SUS doesn't distinguish between files and links in `unlink` so there's no real appropriate value for this
			}
		}
	}

	callCtx, cancel := interfaceutils.CallContext(ki.ctx)
	defer cancel()
	keyName := path[1:]
	if _, err = ki.core.Key().Remove(callCtx, keyName); err != nil {
		return &interfaceutils.Error{
			Cause: err,
			Type:  errors.IO,
		}
	}
	return nil
}
