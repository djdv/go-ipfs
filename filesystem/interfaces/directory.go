package transformcommon

import (
	"context"
	"errors"
	"fmt"
	"sync"

	transform "github.com/ipfs/go-ipfs/filesystem"
)

// TODO: review this garbage; comments, logic, async concerns
// I have no doubt something is wrong

var (
	ErrIsOpen         = errors.New("already open")
	ErrNotOpen        = errors.New("not opened")
	ErrNotInitialized = errors.New("directory not initialized")
)

type ErrorEntry struct{ Err error }

func (ee *ErrorEntry) Name() string   { return "" }
func (ee *ErrorEntry) Offset() uint64 { return 0 }
func (ee *ErrorEntry) Error() error   { return ee.Err }

type PartialEntry interface {
	Name() string
	Error() error
}

type fullEntry struct {
	PartialEntry
	offset uint64
}

func (fe *fullEntry) Offset() uint64 { return fe.offset }

// TODO figure out where to put this nonsense
/*	NOTE:
	While SUSv7 does not specify expected behavior for the case where
	the caller passes an argument to `seekdir` which was not previously obtained from `telldir`;
	our implementation treats this as an error
	and invalidates operations until the directory is reset.
	Where our specific behavior is to return a single entry to the caller,
	who's only valid method is `entry.Error()`, and contains no valid name or offset values.

	Offset values in our system are unique per stream instance,
	and previously valid values become invalid when the stream is `Reset()`
	as such, we validate these offset bounds below.

	The rationale for invalidating previous offsets is to prevent client code
	which depends on unspecified behavior, from appearing to be correct.
	Regardless of if it would succeed incidentally (in the case of IPFS where we could make `Reset()` a no-op),
	or not (in the case of IPNS where entries may have changed between `OpenDir()` and `Reset()`).
	Specifically to guard against this sequence:
	`readdir(dir); off = telldir(); rewinddir(dir); seekdir(off)`
*/
type EntryStorage interface {
	List(context.Context, uint64) <-chan transform.DirectoryEntry
	Reset(<-chan PartialEntry)
}

func NewEntryStorage(streamSource <-chan PartialEntry) EntryStorage {
	return &entryStorage{
		entryStore:   make([]transform.DirectoryEntry, 0),
		sourceStream: streamSource,
	}
}

type entryStorage struct {
	tail         uint64
	entryStore   []transform.DirectoryEntry
	sourceStream <-chan PartialEntry
	sync.WaitGroup
}

func (es *entryStorage) head() uint64 { return es.tail - uint64(len(es.entryStore)) }

func (es *entryStorage) Reset(streamSource <-chan PartialEntry) {
	es.Wait()
	es.Add(1)

	for i := range es.entryStore { // clear the store (so the gc can reap the entries)
		es.entryStore[i] = nil
	}
	es.entryStore = es.entryStore[:0] // reslice it (avoid realloc)

	es.sourceStream = streamSource // tack on the new stream

	// Invalidate the rightmost offset
	// Whether it pointed to a valid entry (on the next read)
	// or was pointing at the end of the stream; we consider it an invalid request past this point
	// The (new) leftmost offset will be based on this (new) boundary value
	// (i.e. the next entry read will be resident/relative 0 of/to the store)
	es.tail++
	es.Done()
}

func (es *entryStorage) List(ctx context.Context, offset uint64) <-chan transform.DirectoryEntry {
	es.Wait()
	es.Add(1)

	// NOTE: offset 0 is a special exception to our lower bound
	// it reads/replays from the beginning of the stream
	cursor := offset
	if offset != 0 {
		// lower bound - provided offset must not be lower than the leftmost stored entry's absolute offset
		// upper bound - our tail, as incremented by each read from the underlying source stream
		if offset < es.head() || offset > es.tail {
			es.Done()
			return errWrap(fmt.Errorf("offset %d is not/no-longer valid", offset))
		}

		// we checked above that the offset value is within our accepted range
		// now we need to do the actual conversion from the absolute value that was previously provided
		// back to a relative index
		// either an index value within the range of cached entries
		// or the head of the stream

		// reduce the provided "absolute offset" to an index of our imaginary boundary [head,tail]
		// then further reduce it relative to the store's boundaries [0,len(store)]
		cursor = (offset % (es.tail + 1)) % uint64(len(es.entryStore)+1)
	}
	// otherwise leave it pointing at tail so we skip over the entries in store
	// i.e. try reading from streams head

	listChan := make(chan transform.DirectoryEntry)

	go func() {
		defer close(listChan)
		defer es.Done()
		// if cursor is within store range, pull entires from it
		if cursor < uint64(len(es.entryStore)) {
			for _, ent := range es.entryStore[cursor:] {
				select {
				case <-ctx.Done(): // list was canceled; we're done
					return

				case listChan <- ent: // relay the entry to the caller
					cursor++ // and move forward
				}
			}
		}

		// entries are not in the store; pull them from the stream
		for {
			select {
			case <-ctx.Done(): // list was canceled; we're done
				return

			default:
				ent, ok := <-es.sourceStream
				if !ok {
					// end of stream
					return
				}

				// attach an offset to the entry and add it to the store
				fullEnt := &fullEntry{ent, es.tail}
				es.tail++
				es.entryStore = append(es.entryStore, fullEnt)

				// between getting the entry and now
				// we may be or have been canceled
				select {
				case <-ctx.Done():
					return
				case listChan <- fullEnt:
				}
			}
		}
	}()
	return listChan
}

func errWrap(err error) <-chan transform.DirectoryEntry {
	errChan := make(chan transform.DirectoryEntry, 1)
	errChan <- &ErrorEntry{err}
	return errChan
}

type PartialStreamSource interface {
	Open() (<-chan PartialEntry, error)
	Close() error
}

type partialStreamWrapper struct {
	PartialStreamSource       // actual source of entries
	EntryStorage              // storage and offset managment for them
	err                 error // errors persis across calls; cleared on Reset
}

func (ps *partialStreamWrapper) Reset() error {
	if err := ps.Close(); err != nil { // invalidate the old stream
		ps.err = err
		return err
	}

	stream, err := ps.Open()
	if err != nil { // get a new stream
		ps.err = err
		return err
	}

	ps.EntryStorage.Reset(stream) // reset the entry store

	ps.err = nil // clear error state, if any
	return nil
}

func (ps *partialStreamWrapper) List(ctx context.Context, offset uint64) <-chan transform.DirectoryEntry {
	if ps.err != nil { // refuse to operate
		return errWrap(ps.err)
	}

	if ps.EntryStorage == nil {
		err := ErrNotInitialized
		ps.err = err
		return errWrap(err)
	}
	return ps.EntryStorage.List(ctx, offset)
}

func PartialEntryUpgrade(streamSource PartialStreamSource) (transform.Directory, error) {
	stream, err := streamSource.Open()
	if err != nil {
		return nil, err
	}

	return &partialStreamWrapper{
		PartialStreamSource: streamSource,
		EntryStorage:        NewEntryStorage(stream),
	}, nil
}
