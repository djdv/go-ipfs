package fsnodes

import (
	"context"
	"crypto/rand"
	"fmt"
	"hash/fnv"
	"io"
	"time"

	"github.com/djdv/p9/p9"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"
	"github.com/ipfs/go-unixfs"
	unixpb "github.com/ipfs/go-unixfs/pb"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

const (
	// TODO [2019.09.12; anyone]
	// Start a discussion around block sizes
	// should we use the de-facto standard of 4KiB or use our own of 256KiB?
	// context: https://github.com/ipfs/go-ipfs/pull/6612/files#r322989041
	ipfsBlockSize = 256 << 10
	saltSize      = 32
)

// NOTE [2019.09.12]: QID's have a high collision probability
// as a result we add a salt to hashes to attempt to mitigate this
// for more context see: https://github.com/ipfs/go-ipfs/pull/6612#discussion_r321038649
var salt []byte

func init() {
	salt = make([]byte, saltSize)
	_, err := io.ReadFull(rand.Reader, salt)
	if err != nil {
		panic(err)
	}
}

func shouldClone(names []string) bool {
	switch len(names) {
	case 0: // empty path
		return true
	case 1: // self?
		pc := names[0]
		return pc == ".." || pc == "." || pc == ""
	default:
		return false
	}
}

func ipldStat(ctx context.Context, attr *p9.Attr, node ipld.Node, mask p9.AttrMask) (error, p9.AttrMask) {
	var filledAttrs p9.AttrMask
	ufsNode, err := unixfs.ExtractFSNode(node)
	if err != nil {
		return err, filledAttrs
	}

	if mask.Mode {
		tBits, err := unixfsTypeTo9Type(ufsNode.Type())
		if err != nil {
			return err, filledAttrs
		}
		attr.Mode |= tBits
		filledAttrs.Mode = true
	}

	if mask.Blocks {
		//TODO: when/if UFS supports this metadata field, use it instead
		attr.BlockSize, filledAttrs.Blocks = ipfsBlockSize, true
	}

	if mask.Size {
		attr.Size, filledAttrs.Size = ufsNode.FileSize(), true
	}

	//TODO [eventually]: handle time metadata in new UFS format standard

	return nil, filledAttrs
}

func cidToQPath(cid cid.Cid) uint64 {
	hasher := fnv.New64a()
	if _, err := hasher.Write(salt); err != nil {
		panic(err)
	}
	if _, err := hasher.Write(cid.Bytes()); err != nil {
		panic(err)
	}
	return hasher.Sum64()
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
	default:
		return p9.ModeRegular, fmt.Errorf("UFS data type %q was not expected, treating as regular file", ut)
	}
}

func coreEntTo9Ent(coreEnt coreiface.DirEntry) (p9.Dirent, error) {
	entType, err := coreTypeTo9Type(coreEnt.Type)
	if err != nil {
		return p9.Dirent{}, err
	}

	return p9.Dirent{
		Name: coreEnt.Name,
		Type: entType.QIDType(),
		QID: p9.QID{
			Type: entType.QIDType(),
			Path: cidToQPath(coreEnt.Cid),
		},
	}, nil
}

const ( // pedantic POSIX stuff
	S_IROTH p9.FileMode = p9.Read
	S_IWOTH             = p9.Write
	S_IXOTH             = p9.Exec

	S_IRGRP = S_IROTH << 3
	S_IWGRP = S_IWOTH << 3
	S_IXGRP = S_IXOTH << 3

	S_IRUSR = S_IRGRP << 3
	S_IWUSR = S_IWGRP << 3
	S_IXUSR = S_IXGRP << 3

	S_IRWXO = S_IROTH | S_IWOTH | S_IXOTH
	S_IRWXG = S_IRGRP | S_IWGRP | S_IXGRP
	S_IRWXU = S_IRUSR | S_IWUSR | S_IXUSR

	IRWXA = S_IRWXU | S_IRWXG | S_IRWXO            // 0777
	IRXA  = IRWXA &^ (S_IWUSR | S_IWGRP | S_IWOTH) // 0555
)

func defaultRootAttr() (attr p9.Attr, attrMask p9.AttrMask) {
	attr.Mode = p9.ModeDirectory | IRXA
	attr.RDev = dMemory
	attrMask.Mode = true
	attrMask.RDev = true
	attrMask.Size = true
	//timeStamp(&attr, attrMask)
	return attr, attrMask
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

func newIPFSBase(ctx context.Context, path corepath.Resolved, kind p9.QIDType, core coreiface.CoreAPI, logger logging.EventLogger) IPFSBase {
	return IPFSBase{
		Path: path,
		core: core,
		Base: Base{
			Logger: logger,
			Ctx:    ctx,
			Qid: p9.QID{
				Type: kind,
				Path: cidToQPath(path.Cid()),
			},
		},
	}
}

// boundCheck assures operation arguments are valid
// returns true if the caller should return immediately with our values
func boundCheck(offset uint64, length int) (bool, error) {
	switch {
	case offset == uint64(length):
		return true, nil // EOS
	case offset > uint64(length):
		return true, fmt.Errorf("offset %d extends beyond directory bound %d", offset, length)
	default:
		// not at end of stream and okay to continue
		return false, nil
	}
}

// walker acts as a dispatcher for intermediate file systems
// sending individual path component requests to their appropriate target system
// regardless of (file system request) origin
func walker(ref walkRef, names []string) ([]p9.QID, p9.File, error) {
	// clone requests go right back to the caller
	if shouldClone(names) {
		return []p9.QID{ref.QID()}, ref, nil
	}

	var (
		nextRef       walkRef
		qids, subQids []p9.QID
		curFile       p9.File
		err           error
	)

	for len(names) != 0 {
		//prepare to step into next component
		if names[0] == ".." { // climb to parent / leftwards requests
			nextRef = ref.Parent()
		} else { // handle remainder / rightward requests
			nextRef = ref.Child()
		}
		if nextRef == nil {
			return []p9.QID{ref.QID()}, nil, fmt.Errorf("system for target %q is not assigned", names[0])
		}

		// attempt the step
		if subQids, curFile, err = nextRef.Walk(names); err != nil {
			return qids, nil, err
		}

		// we walked forward, prepare for next step
		qids = append(qids, subQids...)
		names = names[1:]
		ref = nextRef

		if len(names) != 0 {
			curFile.Close() // we're not referencing this anymore
		} // leave the last reference alive, for the caller to close
	}

	return qids, curFile, nil
}
