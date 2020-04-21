package transform

import (
	"context"
	"crypto/rand"
	"fmt"
	"hash/fnv"
	"io"

	"github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-cid"
	unixpb "github.com/ipfs/go-unixfs/pb"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

// this file contains all the data transforms for * -> 9P

// TODO: [da5df057-6160-46b9-9a42-b207008076bd] extracted from 9P/utils; we need to evaluate what we want to keep and what to export

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

func coreToQID(ctx context.Context, path corepath.Resolved, core coreiface.CoreAPI) (p9.QID, error) {
	var qid p9.QID

	stat, _, err := GetAttrCore(ctx, path, core, IPFSStatRequest{Type: true})
	if err != nil {
		return qid, err
	}

	attr := stat.To9P()

	qid.Type = attr.Mode.QIDType()
	qid.Path = cidToQIDPath(path.Cid())
	return qid, nil
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

func unixfsTypeTo9Type(ut unixpb.Data_DataType) (p9.FileMode, error) {
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

func coreDirEntryTo9Dirent(coreEnt coreiface.DirEntry) p9.Dirent {
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
