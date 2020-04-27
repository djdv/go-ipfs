package transform

import (
	"runtime"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/hugelgupf/p9/p9"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

type IPFSStat struct {
	FileType  coreiface.FileType
	Size      uint64
	BlockSize uint64
	Blocks    uint64
	/* TODO: UFS 2 when it's done
	ATimeNano int64
	MTimeNano int64
	CTimeNano int64 */
}

var IPFSStatRequestAll = IPFSStatRequest{
	Type: true, Size: true, Blocks: true,
}

type IPFSStatRequest struct {
	Type   bool
	Size   bool
	Blocks bool
	/* TODO: UFS 2 when it's done
	ATime       bool
	MTime       bool
	CTime       bool
	*/
}

func (cs *IPFSStat) ToFuse() *fuselib.Stat_t {
	// TODO [safety] we should probably panic if the uint64 source values exceed int64 positive range

	if runtime.GOOS == "windows" {
		if cs.FileType == coreiface.TSymlink {
			return &fuselib.Stat_t{
				Mode:  fuselib.S_IFLNK,
				Flags: fuselib.UF_ARCHIVE, // this is conventional `mklink` will set this attribute
				// NOTE: size omitted for the same reason
			}
		}
	}

	return &fuselib.Stat_t{
		Mode:    coreTypeToFuseType(cs.FileType),
		Size:    int64(cs.Size),
		Blksize: int64(cs.BlockSize),
		Blocks:  int64(cs.Blocks),
	}
}

func (cs *IPFSStat) To9P() *p9.Attr {
	// TODO [safety] we should probably panic if the uint64 source values exceed int64 positive range
	return &p9.Attr{
		Mode:      coreTypeTo9PType(cs.FileType),
		Size:      cs.Size,
		BlockSize: cs.BlockSize,
		Blocks:    cs.Blocks,
	}
}
