package transform

import (
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/hugelgupf/p9/p9"
)

// TODO: local errors -> common error package, maybe under mount interface/errors.go?
const errSeekFmt = "offset %d extends beyond directory bound %d"

type Directory interface {
	// Read returns /at most/ count entries; or attempts to return all entires when count is 0
	Read(offset, count uint64) directoryState
	Seek(offset uint64) error
	Close() error
}

type FuseStatGroup struct {
	Name   string
	Offset int64
	Stat   *fuselib.Stat_t
}

// TODO: better name
type directoryState interface {
	// TODO: ToGo() ([]os.Fileinfo, error)
	To9P() (p9.Dirents, error)
	ToFuse() (<-chan FuseStatGroup, error)
}
