package keyfs

import (
	transform "github.com/ipfs/go-ipfs/filesystem"
	tcom "github.com/ipfs/go-ipfs/filesystem/interfaces"
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
