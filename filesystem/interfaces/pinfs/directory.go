package pinfs

import (
	transform "github.com/ipfs/go-ipfs/filesystem"
	tcom "github.com/ipfs/go-ipfs/filesystem/interfaces"
)

func (pi *pinInterface) OpenDirectory(path string) (transform.Directory, error) {
	if path == "/" {
		return tcom.PartialEntryUpgrade(
			tcom.NewStreamBase(pi.ctx, &pinDirectoryStream{pinAPI: pi.core.Pin()}))
	}

	return pi.ipfs.OpenDirectory(path)
}
