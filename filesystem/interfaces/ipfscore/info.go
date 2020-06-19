package ipfscore

import (
	"fmt"

	files "github.com/ipfs/go-ipfs-files"
	transform "github.com/ipfs/go-ipfs/filesystem"
	transcom "github.com/ipfs/go-ipfs/filesystem/interfaces"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var (
	rootStat   = &transform.IPFSStat{FileType: coreiface.TDirectory}
	rootFilled = transform.IPFSStatRequest{Type: true}
)

func (ci *coreInterface) Info(path string, req transform.IPFSStatRequest) (*transform.IPFSStat, transform.IPFSStatRequest, error) {
	if path == "/" {
		return rootStat, rootFilled, nil
	}

	callCtx, cancel := transcom.CallContext(ci.ctx)
	defer cancel()
	return ci.core.Stat(callCtx, ci.joinRoot(path), req)
}
func (ci *coreInterface) ExtractLink(path string) (string, error) {
	// make sure the path is actually a link
	iStat, _, err := ci.Info(path, transform.IPFSStatRequest{Type: true})
	if err != nil {
		return "", err
	}

	if iStat.FileType != coreiface.TSymlink {
		return "", &transcom.Error{
			Cause: fmt.Errorf("%q is not a symlink", path),
			Type:  transform.ErrorInvalidItem,
		}
	}

	// if it is, read it
	callCtx, callCancel := transcom.CallContext(ci.ctx)
	defer callCancel()
	linkNode, err := ci.core.Unixfs().Get(callCtx, ci.joinRoot(path))
	if err != nil {
		return "", &transcom.Error{
			Cause: err,
			Type:  transform.ErrorIO,
		}
	}

	// NOTE: the implementation of this does no type checks [2020.06.04]
	// which is why we check the node's type above
	return files.ToSymlink(linkNode).Target, nil
}
