package transform

import (
	"crypto/rand"
	"fmt"
	"hash/fnv"
	"io"

	"github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-cid"
	unixpb "github.com/ipfs/go-unixfs/pb"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

func init() {
	qidGeneratorSalt = make([]byte, saltSize)
	_, err := io.ReadFull(rand.Reader, qidGeneratorSalt)
	if err != nil {
		panic(err)
	}
}

// NOTE [2019.09.12]: QID's have a high collision probability
// as a result we add a salt to hashes to attempt to mitigate this
// for more context see: https://github.com/ipfs/go-ipfs/pull/6612#discussion_r321038649
const saltSize = 32

var qidGeneratorSalt []byte

//NOTE [2019.09.11]: IPFS CoreAPI abstracts over HAMT structures; Unixfs returns raw type

func UnixfsTypeToCoreType(ut unixpb.Data_DataType) coreiface.FileType {
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

// NOTE: copied [57d3a09e-bf0a-41d3-8d9d-ff70690e8624]
// this is probably fine but we may want to export all of these kinds of methods in the transform layer
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

func CoreDirEntryTo9Dirent(coreEnt coreiface.DirEntry) p9.Dirent {
	entType := coreTypeTo9PType(coreEnt.Type)
	return p9.Dirent{
		Name: coreEnt.Name,
		Type: entType.QIDType(),
		QID: p9.QID{
			Type: entType.QIDType(),
			Path: cidToQIDPath(coreEnt.Cid),
		},
	}
}

func UnixfsTypeTo9Type(ut unixpb.Data_DataType) (p9.FileMode, error) {
	switch ut {
	//TODO: directories and hamt shards are not synonymous; HAMTs may need special handling
	case unixpb.Data_Directory, unixpb.Data_HAMTShard:
		return p9.ModeDirectory, nil
	case unixpb.Data_Symlink:
		return p9.ModeSymlink, nil
	case unixpb.Data_File:
		return p9.ModeRegular, nil
	case unixpb.Data_Raw: //TODO [investigate]: the result of `mfs.WriteAt` produces a file of this type if the contents are small enough
		return p9.ModeRegular, nil
	default:
		return p9.ModeRegular, fmt.Errorf("UFS data type %q was not expected, treating as regular file", ut)
	}
}

func CidToQID(cid cid.Cid, ct coreiface.FileType) p9.QID {
	return p9.QID{
		Type: coreTypeTo9PType(ct).QIDType(),
		Path: cidToQIDPath(cid),
	}
}

func cidToQIDPath(cid cid.Cid) uint64 {
	hasher := fnv.New64a()
	if _, err := hasher.Write(qidGeneratorSalt); err != nil {
		panic(err)
	}
	if _, err := hasher.Write(cid.Bytes()); err != nil {
		panic(err)
	}
	return hasher.Sum64()
}
