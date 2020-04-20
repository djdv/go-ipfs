package transform

import (
	"context"
	"crypto/rand"
	"hash/fnv"
	"io"

	"github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-cid"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

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
