package ufs

import (
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

var (
	rootStat   = &transform.IPFSStat{FileType: coreiface.TDirectory}
	rootFilled = transform.IPFSStatRequest{Type: true}
)

func (ui *ufsInterface) Info(path string, req transform.IPFSStatRequest) (attr *transform.IPFSStat, filled transform.IPFSStatRequest, err error) {
	if path == "/" {
		return rootStat, rootFilled, nil
	}

	return ui.core.Stat(ui.ctx, corepath.New(path), req)
}

func (ui *ufsInterface) ExtractLink(path string) (string, error) {
	return ui.core.ExtractLink(corepath.New(path))
}
