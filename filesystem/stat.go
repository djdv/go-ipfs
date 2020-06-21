package filesystem

import (
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
