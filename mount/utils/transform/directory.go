package transform

import (
	"context"
	"io"
	"os"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/hugelgupf/p9/p9"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

// TODO: local errors -> common error package, maybe under mount interface/errors.go?
const errSeekFmt = "offset %d extends beyond directory bound %d"

type Directory interface {
	// Readdir returns attempts to return all entires starting from offset until it reaches the end
	// or the context is canceled
	Readdir(ctx context.Context, offset uint64) DirectoryState
	// TODO: ^ rename to List
	// TODO: implement
	// Reset() error
	io.Closer
}

// TODO: remove when implementations have transitioned to new Directory format
type DirectoryStream interface {
	Directory
	DontReset()
}

type StreamSource interface {
	Open() (<-chan DirectoryStreamEntry, error)
	Close() error
}

type DirectoryStreamEntry interface {
	Name() string
	Path() corepath.Path // remove this, add offset
	Error() error
}

type FuseStatGroup struct {
	Name   string
	Offset int64
	Stat   *fuselib.Stat_t // remove this; formalize Stream entry as the standard with an offset
}

// TODO: remove this; overly complicates things for no real benefit
// translation of entries should be done at the FS layer
type DirectoryState interface {
	// TODO: for Go and 9P, allow the user to pass in a pre-allocated slice (or nil)
	// same for Fuse but with a channel, in case they want it buffered
	// NOTE: pre-allocated/defined inputs are optional and should be allocated internally if nil
	// channels must be closed by the method
	//To9P() (p9.Dirents, error)
	To9P(count uint32) (p9.Dirents, error)
	ToGo() ([]os.FileInfo, error)
	ToGoC(predefined chan os.FileInfo) (<-chan os.FileInfo, error)
	ToFuse() (<-chan FuseStatGroup, error)
}
