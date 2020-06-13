package keyfs

import (
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	tcom "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems"
)

func (ki *keyInterface) OpenDirectory(path string) (transform.Directory, error) {
	fs, _, fsPath, deferFunc, err := ki.selectFS(path)
	if err != nil {
		return nil, err
	}
	defer deferFunc()

	if fs == ki {
		return tcom.PartialEntryUpgrade(
			tcom.NewStreamBase(ki.ctx, &keyDirectoryStream{keyAPI: ki.core.Key()}))
	}

	return fs.OpenDirectory(fsPath)
}
