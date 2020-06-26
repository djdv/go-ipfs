package filesystem

import (
	"context"
	"io"
)

type Directory interface {
	// List attempts to return all entires starting from `offset`
	// `offset` values must be either 0 or a value previously provided by `DirectoryEntry.Offset()`
	// the returned channel is closed under the following conditions:
	//  - The context is canceled
	//  - The end of the listing is reached
	//  - An error was encountered during listing
	// if an error is encountered during listing, an entry is returned containing it
	// (prior to closing the channel)
	List(ctx context.Context, offset uint64) <-chan DirectoryEntry
	// Reset will cause the `Directory` to reinitialize itself
	// TODO: this might be better named `Refresh`
	// we also need to better define its purpose and relation to `List`
	// it was needed to mimic SUS's `rewinddir`
	// but we kind of don't want that to be tied to the interface so much as the implementations
	Reset() error
	io.Closer
}

// DirectoryEntry contains basic information about an entry within a directory
// returned from `List`, it specifies the offset value for the next entry to be listed
// you may provide this value to `List` if you wish to resume an interuppted listing
// or replay a listing, from this entry
// TODO: document all the nonsense around offset values and how they relate to `Reset`
// (^ just some note about implementation specific blah blah)
type DirectoryEntry interface {
	Name() string
	Offset() uint64
	Error() error
}
