package filesystem

import (
	"context"
	"io"
)

// TODO: local errors -> common error package, maybe under mount interface/errors.go?
const errSeekFmt = "offset %d extends beyond directory bound %d"

type Directory interface {
	// List attempts to return all entires starting from offset until it reaches the end
	// or the context is canceled
	// offset values must be either 0 or a value previously provided by DirectoryEntry.Offset()
	// if an error is encountered, an entry is returned containing it, and the channel is closed
	List(ctx context.Context, offset uint64) <-chan DirectoryEntry
	// Reset
	Reset() error // TODO: should this be a transform error? most likely
	io.Closer
}

type DirectoryEntry interface {
	Name() string
	Offset() uint64
	Error() error // TODO: should this be a transform error? most likely
}
