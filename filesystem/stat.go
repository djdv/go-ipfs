package transform

import (
	"github.com/hugelgupf/p9/p9"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

// TODO: [lex] drop prefix
type IPFSStat struct {
	FileType  coreiface.FileType // TODO: [lex] drop File prefix
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

// TODO: [lex] drop prefix
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

// TODO: expunge
func (cs *IPFSStat) To9P() p9.Attr {
	// TODO [safety] we should probably panic if the uint64 source values exceed int64 positive range
	return p9.Attr{
		Mode:      coreTypeTo9PType(cs.FileType),
		Size:      cs.Size,
		BlockSize: cs.BlockSize,
		Blocks:    cs.Blocks,
	}
}
