package transform

import (
	unixpb "github.com/ipfs/go-unixfs/pb"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

//NOTE [2019.09.11]: IPFS CoreAPI abstracts over HAMT structures; Unixfs returns raw type

func unixfsTypeToCoreType(ut unixpb.Data_DataType) coreiface.FileType {
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
