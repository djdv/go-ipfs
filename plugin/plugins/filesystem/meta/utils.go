package meta

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"time"

	"github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-mfs"
	"github.com/ipfs/go-unixfs"
	unixpb "github.com/ipfs/go-unixfs/pb"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
	"github.com/multiformats/go-multihash"
)

// NOTE [2019.09.12]: QID's have a high collision probability
// as a result we add a salt to hashes to attempt to mitigate this
// for more context see: https://github.com/ipfs/go-ipfs/pull/6612#discussion_r321038649
const saltSize = 32

var qidGeneratorSalt []byte

func init() {
	qidGeneratorSalt = make([]byte, saltSize)
	_, err := io.ReadFull(rand.Reader, qidGeneratorSalt)
	if err != nil {
		panic(err)
	}
}

func CidToQIDPath(cid cid.Cid) uint64 {
	hasher := fnv.New64a()
	if _, err := hasher.Write(qidGeneratorSalt); err != nil {
		panic(err)
	}
	if _, err := hasher.Write(cid.Bytes()); err != nil {
		panic(err)
	}
	return hasher.Sum64()
}

func CoreToQID(ctx context.Context, path corepath.Path, core coreiface.CoreAPI) (p9.QID, error) {
	var qid p9.QID
	// translate from abstract path to CoreAPI resolved path
	resolvedPath, err := core.ResolvePath(ctx, path)
	if err != nil {
		return qid, err
	}

	// inspected to derive 9P QID
	attr := new(p9.Attr)
	_, err = CoreToAttr(ctx, attr, resolvedPath, core, p9.AttrMask{Mode: true})
	if err != nil {
		return qid, err
	}

	qid.Type = attr.Mode.QIDType()
	qid.Path = CidToQIDPath(resolvedPath.Cid())
	return qid, nil
}

func CoreToAttr(ctx context.Context, attr *p9.Attr, path corepath.Path, core coreiface.CoreAPI, req p9.AttrMask) (p9.AttrMask, error) {
	// translate from abstract path to CoreAPI resolved path
	resolvedPath, err := core.ResolvePath(ctx, path)
	if err != nil {
		return p9.AttrMask{}, err
	}

	ipldNode, err := core.Dag().Get(ctx, resolvedPath.Cid())
	if err != nil {
		return p9.AttrMask{}, err
	}

	return IpldStat(ctx, attr, ipldNode, req)
}

func IpldStat(ctx context.Context, attr *p9.Attr, node ipld.Node, mask p9.AttrMask) (p9.AttrMask, error) {
	var filledAttrs p9.AttrMask
	ufsNode, err := unixfs.ExtractFSNode(node)
	if err != nil {
		return filledAttrs, err
	}

	if mask.Mode {
		tBits, err := unixfsTypeTo9Type(ufsNode.Type())
		if err != nil {
			return filledAttrs, err
		}
		attr.Mode = tBits
		filledAttrs.Mode = true
	}

	if mask.Blocks {
		//TODO: when/if UFS supports this metadata field, use it instead
		attr.BlockSize, filledAttrs.Blocks = UFS1BlockSize, true
	}

	if mask.Size {
		attr.Size, filledAttrs.Size = ufsNode.FileSize(), true
	}

	//TODO [eventually]: handle time metadata in new UFS format standard

	return filledAttrs, nil
}

type RootPath string

func (rp RootPath) String() string { return string(rp) }
func (RootPath) Namespace() string { return "" } // root namespace is intentionally left blank
func (RootPath) Mutable() bool     { return true }
func (RootPath) IsValid() error    { return nil }
func (RootPath) Root() cid.Cid     { return cid.Cid{} }
func (RootPath) Remainder() string { return "" }
func (rp RootPath) Cid() cid.Cid {
	prefix := cid.V1Builder{Codec: cid.DagCBOR, MhType: multihash.BLAKE2B_MIN}
	c, err := prefix.Sum([]byte(rp))
	if err != nil {
		panic(err) //invalid root
	}
	return c
}

//NOTE [2019.09.11]: IPFS CoreAPI abstracts over HAMT structures; Unixfs returns raw type

func coreTypeTo9Type(ct coreiface.FileType) (p9.FileMode, error) {
	switch ct {
	case coreiface.TDirectory:
		return p9.ModeDirectory, nil
	case coreiface.TSymlink:
		return p9.ModeSymlink, nil
	case coreiface.TFile:
		return p9.ModeRegular, nil
	default:
		return p9.ModeRegular, fmt.Errorf("CoreAPI data type %q was not expected, treating as regular file", ct)
	}
}

//TODO: see if we can remove the need for this; rely only on the core if we can
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

func CoreEntTo9Ent(coreEnt coreiface.DirEntry) (p9.Dirent, error) {
	entType, err := coreTypeTo9Type(coreEnt.Type)
	if err != nil {
		return p9.Dirent{}, err
	}

	return p9.Dirent{
		Name: coreEnt.Name,
		Type: entType.QIDType(),
		QID: p9.QID{
			Type: entType.QIDType(),
			Path: CidToQIDPath(coreEnt.Cid),
		},
	}, nil
}

func MFSTypeToNineType(nt mfs.NodeType) (entType p9.QIDType, err error) {
	switch nt {
	//mfsEnt.Type; mfs.NodeType(t) {
	case mfs.TFile:
		entType = p9.TypeRegular
	case mfs.TDir:
		entType = p9.TypeDir
	default:
		err = fmt.Errorf("unexpected node type %v", nt)
	}
	return
}

func MFSEntTo9Ent(mfsEnt mfs.NodeListing) (p9.Dirent, error) {
	pathCid, err := cid.Decode(mfsEnt.Hash)
	if err != nil {
		return p9.Dirent{}, err
	}

	t, err := MFSTypeToNineType(mfs.NodeType(mfsEnt.Type))
	if err != nil {
		return p9.Dirent{}, err
	}

	return p9.Dirent{
		Name: mfsEnt.Name,
		Type: t,
		QID: p9.QID{
			Type: t,
			Path: CidToQIDPath(pathCid),
		},
	}, nil
}

func timeStamp(attr *p9.Attr, mask p9.AttrMask) {
	now := time.Now()
	if mask.ATime {
		attr.ATimeSeconds, attr.ATimeNanoSeconds = uint64(now.Unix()), uint64(now.UnixNano())
	}
	if mask.MTime {
		attr.MTimeSeconds, attr.MTimeNanoSeconds = uint64(now.Unix()), uint64(now.UnixNano())
	}
	if mask.CTime {
		attr.CTimeSeconds, attr.CTimeNanoSeconds = uint64(now.Unix()), uint64(now.UnixNano())
	}
}

// FlatReader takes in a slice of 9P entries, and returns the appropriate values for Readdir response messages
func FlatReaddir(ents []p9.Dirent, offset uint64, count uint32) ([]p9.Dirent, error) {
	switch l := uint64(len(ents)); {
	case l == offset:
		return nil, nil
	case l < offset:
		return nil, fmt.Errorf("offset %d extends beyond directory bound %d", offset, l)
	}

	subSlice := ents[offset:]
	if len(subSlice) > int(count) {
		subSlice = subSlice[:count]
	}
	return subSlice, nil
}

func CidToMFSRoot(ctx context.Context, rootCid cid.Cid, core coreiface.CoreAPI, publish mfs.PubFunc) (*mfs.Root, error) {
	if !rootCid.Defined() {
		return nil, errors.New("root cid was not defined")
	}

	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ipldNode, err := core.Dag().Get(callCtx, rootCid)
	if err != nil {
		return nil, err
	}

	pbNode, ok := ipldNode.(*dag.ProtoNode)
	if !ok {
		return nil, fmt.Errorf("%q has incompatible type %T", rootCid.String(), ipldNode)
	}

	return mfs.NewRoot(ctx, core.Dag(), pbNode, publish)
}
