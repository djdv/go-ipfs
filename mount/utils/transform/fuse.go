package transform

import (
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

type fuseFileType = uint32

func coreTypeToFuseType(ct coreiface.FileType) fuseFileType {
	switch ct {
	case coreiface.TDirectory:
		return fuselib.S_IFDIR
	case coreiface.TSymlink:
		return fuselib.S_IFLNK
	case coreiface.TFile:
		return fuselib.S_IFREG
	default:
		return 0
	}
}
