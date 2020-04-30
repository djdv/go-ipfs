package transform

import (
	"io"
	"os"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/hugelgupf/p9/p9"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

// TODO: local errors -> common error package, maybe under mount interface/errors.go?
const errSeekFmt = "offset %d extends beyond directory bound %d"

type Directory interface {
	// Read returns /at most/ count entries; or attempts to return all entires when count is 0
	Readdir(offset, count uint64) DirectoryState
	io.Closer
}

type StreamSource interface {
	Open() (<-chan DirectoryStreamEntry, error)
	Close() error
}

type DirectoryStreamEntry interface {
	Name() string
	Path() corepath.Path
	Error() error
}

type FuseStatGroup struct {
	Name   string
	Offset int64
	Stat   *fuselib.Stat_t
}

// TODO: better name
type DirectoryState interface {
	// TODO: for Go and 9P, allow the user to pass in a pre-allocated slice (or nil)
	// same for Fuse but with a channel, in case they want it buffered
	// NOTE: pre-allocated/defined inputs are optional and should be allocated internally if nil
	// channels must be closed by the method
	To9P() (p9.Dirents, error)
	ToGo() ([]os.FileInfo, error)
	ToGoC(predefined chan os.FileInfo) (<-chan os.FileInfo, error)
	ToFuse() (<-chan FuseStatGroup, error)
}
