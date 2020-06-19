package pinfs

import (
	transform "github.com/ipfs/go-ipfs/filesystem"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var (
	rootStat   = &transform.IPFSStat{FileType: coreiface.TDirectory}
	rootFilled = transform.IPFSStatRequest{Type: true}
)

func (pi *pinInterface) Info(path string, req transform.IPFSStatRequest) (*transform.IPFSStat, transform.IPFSStatRequest, error) {
	if path == "/" {
		return rootStat, rootFilled, nil
	}
	return pi.ipfs.Info(path, req)
}

func (pi *pinInterface) ExtractLink(path string) (string, error) { return pi.ipfs.ExtractLink(path) }
