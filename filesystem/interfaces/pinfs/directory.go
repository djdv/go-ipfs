package pinfs

import (
	"github.com/ipfs/go-ipfs/filesystem"
	tcom "github.com/ipfs/go-ipfs/filesystem/interfaces"
)

func (pi *pinInterface) OpenDirectory(path string) (filesystem.Directory, error) {
	if path == "/" {
		return tcom.UpgradePartialStream(
			tcom.NewPartialStream(pi.ctx, &pinDirectoryStream{pinAPI: pi.core.Pin()}))
	}

	return pi.ipfs.OpenDirectory(path)
}
