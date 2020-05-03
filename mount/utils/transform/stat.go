package transform

import (
	"os"
	"runtime"
	"time"

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

func RequestFrom9P(req p9.AttrMask) IPFSStatRequest {
	var iReq IPFSStatRequest
	if req.Mode {
		iReq.Type = true
	}
	if req.Size {
		iReq.Size = true
	}
	if iReq.Blocks {
		iReq.Blocks = true
	}
	return iReq
}

func (sr *IPFSStatRequest) To9P() (filled p9.AttrMask) {
	if sr.Type {
		filled.Mode = true
	}
	if sr.Size {
		filled.Size = true
	}
	if sr.Blocks {
		filled.Blocks = true
	}
	return
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

func (cs *IPFSStat) To9P() p9.Attr {
	// TODO [safety] we should probably panic if the uint64 source values exceed int64 positive range
	return p9.Attr{
		Mode:      coreTypeTo9PType(cs.FileType),
		Size:      cs.Size,
		BlockSize: cs.BlockSize,
		Blocks:    cs.Blocks,
	}
}

func (cs *IPFSStat) ToGo(name string) os.FileInfo {
	// TODO [safety] we should probably panic if the uint64 source values exceed int64 positive range
	return &goWrapper{
		sys:  cs,
		name: name,
		mode: coreTypeToGoType(cs.FileType),
		size: int64(cs.Size),
	}
}

type goWrapper struct {
	sys  *IPFSStat
	name string
	size int64
	mode os.FileMode
}

func (gw *goWrapper) Name() string       { return gw.name }
func (gw *goWrapper) Size() int64        { return gw.size }
func (gw *goWrapper) Mode() os.FileMode  { return gw.mode }
func (gw *goWrapper) ModTime() time.Time { return time.Time{} }
func (gw *goWrapper) IsDir() bool        { return gw.mode.IsDir() }
func (gw *goWrapper) Sys() interface{}   { return gw.sys }
