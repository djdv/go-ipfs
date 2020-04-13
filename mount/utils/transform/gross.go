package transform

import (
	"context"
	"fmt"

	"github.com/hugelgupf/p9/p9"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-unixfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
	fuselib "github.com/billziss-gh/cgofuse/fuse"
)

func unixfsTypeToCoreType(ut unixpb.Data_DataType) coreiface.FileType{
	switch ut {
	// TODO: directories and hamt shards are not synonymous; HAMTs may need special handling
	case unixpb.Data_Directory, unixpb.Data_HAMTShard:
		return coreiface.TDirectory
	case unixpb.Data_Symlink:
		return coreiface.TSymlink
	// TODO: files and raw data are not synonymous; `mfs.WriteAt` will produce a file of this type however if the contents are small enough
	case unixpb.Data_File, unixpb.Data_Raw:
		return coreiface.TFile
	default:
		return coreiface.TUnknown
	}
}

func coreTypeTo9PType(ct coreiface.FileType) p9.FileMode {
	switch ct {
	case coreiface.TDirectory:
		return p9.ModeDirectory
	case coreiface.TSymlink:
		return p9.ModeSymlink
	case coreiface.TFile:
		return p9.ModeRegular
	default:
		return p9.FileMode(0)
	}
}

type fuseFileType = uint32

func coreTypeToFuseType(ct coreiface.FileType) fuseFileType
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