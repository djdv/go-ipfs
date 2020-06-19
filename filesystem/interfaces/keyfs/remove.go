package keyfs

import (
	"fmt"

	transform "github.com/ipfs/go-ipfs/filesystem"
	transcom "github.com/ipfs/go-ipfs/filesystem/interfaces"
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
	iStat, _, err := ki.Info(path, transform.IPFSStatRequest{Type: true})
	if err != nil {
		return err
	}

	if iStat.FileType != nodeType {
		switch nodeType {
		case coreiface.TFile:
			return &transcom.Error{
				Cause: fmt.Errorf("%q is not a file", path),
				Type:  transform.ErrorIsDir,
			}
		case coreiface.TDirectory:
			return &transcom.Error{
				Cause: fmt.Errorf("%q is not a directory", path),
				Type:  transform.ErrorNotDir,
			}
		case coreiface.TSymlink:
			return &transcom.Error{
				Cause: fmt.Errorf("%q is not a link", path),
				Type:  transform.ErrorNotExist, // TODO: [review] SUS doesn't distinguish between files and links in `unlink` so there's no real appropriate value for this
			}
		}
	}

	callCtx, cancel := transcom.CallContext(ki.ctx)
	defer cancel()
	keyName := path[1:]
	if _, err = ki.core.Key().Remove(callCtx, keyName); err != nil {
		return &transcom.Error{
			Cause: err,
			Type:  transform.ErrorIO,
		}
	}
	return nil
}
