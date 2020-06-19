package keyfs

import (
	"errors"

	transform "github.com/ipfs/go-ipfs/filesystem"
	transcom "github.com/ipfs/go-ipfs/filesystem/interfaces"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var (
	rootStat   = &transform.IPFSStat{FileType: coreiface.TDirectory}
	rootFilled = transform.IPFSStatRequest{Type: true}
)

func (ki *keyInterface) Info(path string, req transform.IPFSStatRequest) (*transform.IPFSStat, transform.IPFSStatRequest, error) {
	fs, key, fsPath, deferFunc, err := ki.selectFS(path)
	if err != nil {
		return nil, transform.IPFSStatRequest{}, &transcom.Error{Cause: err, Type: transform.ErrorOther}
	}
	defer deferFunc()

	if fs == ki {
		if fsPath == "/" {
			return rootStat, rootFilled, nil
		}
		callCtx, cancel := transcom.CallContext(ki.ctx)
		defer cancel()
		return ki.core.Stat(callCtx, key.Path(), req)
	}
	return fs.Info(fsPath, req)
}

func (ki *keyInterface) ExtractLink(path string) (string, error) {
	if path == "/" {
		return "", &transcom.Error{Cause: errors.New("root is not a link"), Type: transform.ErrorInvalidItem}
	}

	fs, key, fsPath, deferFunc, err := ki.selectFS(path)
	if err != nil {
		return "", &transcom.Error{Cause: err, Type: transform.ErrorOther}
	}
	defer deferFunc()

	if fs == ki {
		return ki.core.ExtractLink(key.Path())
	}
	return fs.ExtractLink(fsPath)
}
