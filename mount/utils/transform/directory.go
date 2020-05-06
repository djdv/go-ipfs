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
	io.Closer
}

// TODO: review; [kludge] if there's a better way to handle this, do it
// This interface provides a way for stream wrappers to prevent a stream from resetting itself
// upon the next call to `Readdir`.
// Allowing a stream handler to replay a stream from the 0th element by calling
// `stream.DontReset()`; `stream.Readdir(0,0)`
// This is mainly necessary due to an ambiguity in FUSE
// which effectively obfuscates/conflates `seekdir(0)` and `rewinddir` at our implementation layer.
// in the event a stream is not yet initialized, it proceeds as if `DontReset` was not called
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
	//To9P() (p9.Dirents, error)
	To9P(count uint32) (p9.Dirents, error)
	ToGo() ([]os.FileInfo, error)
	ToGoC(predefined chan os.FileInfo) (<-chan os.FileInfo, error)
	ToFuse() (<-chan FuseStatGroup, error)
}
