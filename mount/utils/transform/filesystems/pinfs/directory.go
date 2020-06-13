package pinfs

import (
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	tcom "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems"
)

func (pi *pinInterface) OpenDirectory(path string) (transform.Directory, error) {
	if path == "/" {
		return tcom.PartialEntryUpgrade(
			tcom.NewStreamBase(pi.ctx, &pinDirectoryStream{pinAPI: pi.core.Pin()}))
	}

	return pi.ipfs.OpenDirectory(path)
}
