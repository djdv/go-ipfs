package ipfscore

import (
	"fmt"

	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-ipfs/filesystem"
	fserrors "github.com/ipfs/go-ipfs/filesystem/errors"
	interfaceutils "github.com/ipfs/go-ipfs/filesystem/interface"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var (
	rootStat   = &filesystem.Stat{Type: coreiface.TDirectory}
	rootFilled = filesystem.StatRequest{Type: true}
)

func (ci *coreInterface) Info(path string, req filesystem.StatRequest) (*filesystem.Stat, filesystem.StatRequest, error) {
	if path == "/" {
		return rootStat, rootFilled, nil
	}

	callCtx, cancel := interfaceutils.CallContext(ci.ctx)
	defer cancel()
	return ci.core.Stat(callCtx, ci.joinRoot(path), req)
}

func (ci *coreInterface) ExtractLink(path string) (string, error) {
	// make sure the path is actually a link
	iStat, _, err := ci.Info(path, filesystem.StatRequest{Type: true})
	if err != nil {
		return "", err
	}

	if iStat.Type != coreiface.TSymlink {
		return "", &interfaceutils.Error{
			Cause: fmt.Errorf("%q is not a symlink", path),
			Type:  fserrors.InvalidItem,
		}
	}

	// if it is, read it
	callCtx, callCancel := interfaceutils.CallContext(ci.ctx)
	defer callCancel()
	linkNode, err := ci.core.Unixfs().Get(callCtx, ci.joinRoot(path))
	if err != nil {
		return "", &interfaceutils.Error{
			Cause: err,
			Type:  fserrors.IO,
		}
	}

	// NOTE: the implementation of this does no type checks [2020.06.04]
	// which is why we check the node's type above
	return files.ToSymlink(linkNode).Target, nil
}
