package keyfs

import (
	"errors"

	"github.com/ipfs/go-ipfs/filesystem"
	interfaceutils "github.com/ipfs/go-ipfs/filesystem/interface"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var (
	rootStat   = &filesystem.Stat{Type: coreiface.TDirectory}
	rootFilled = filesystem.StatRequest{Type: true}
)

func (ki *keyInterface) Info(path string, req filesystem.StatRequest) (*filesystem.Stat, filesystem.StatRequest, error) {
	fs, key, fsPath, deferFunc, err := ki.selectFS(path)
	if err != nil {
		return nil, filesystem.StatRequest{}, &interfaceutils.Error{Cause: err, Type: filesystem.ErrorOther}
	}
	defer deferFunc()

	if fs == ki {
		if fsPath == "/" {
			return rootStat, rootFilled, nil
		}
		callCtx, cancel := interfaceutils.CallContext(ki.ctx)
		defer cancel()
		return ki.core.Stat(callCtx, key.Path(), req)
	}
	return fs.Info(fsPath, req)
}

func (ki *keyInterface) ExtractLink(path string) (string, error) {
	if path == "/" {
		return "", &interfaceutils.Error{Cause: errors.New("root is not a link"), Type: filesystem.ErrorInvalidItem}
	}

	fs, key, fsPath, deferFunc, err := ki.selectFS(path)
	if err != nil {
		return "", &interfaceutils.Error{Cause: err, Type: filesystem.ErrorOther}
	}
	defer deferFunc()

	if fs == ki {
		return ki.core.ExtractLink(key.Path())
	}
	return fs.ExtractLink(fsPath)
}
