package ipfscore

import (
	"context"
	"errors"

	"github.com/ipfs/go-ipfs/mount/utils/transform"
	tcom "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

// TODO: make a pass on the overly verbose nonsense comments [PD AM ENG]
// also all the context cancels and error conditions
// Also large refactor so comments are probably flat out wrong in some places

var _ transform.Directory = (*coreDirectoryStream)(nil)

type coreDirectoryStream struct {
	openCtx, coreCtx context.Context // top level and operation context
	coreCancel       context.CancelFunc
	err              error // errors persist until the stream is reset

	// stream replay buffer; allows for seeking to previous points in the stream
	tcom.EntryStorage

	// HACK: [d21a38b9-e723-4068-ad72-7473b91cc770] try to ignore Reset requests that happen immediately after an open
	justOpened bool

	core coreiface.CoreAPI // used during stream source construction and listing
	path corepath.Path     // the streams source location; used to (re)construct the stream in `Open`/`Reset`
}

// OpenDir returns a Directory for the given path (as a stream of entries)
func OpenDir(ctx context.Context, path corepath.Path, core coreiface.CoreAPI) (*coreDirectoryStream, error) {
	coreStream := &coreDirectoryStream{
		openCtx:    ctx,
		core:       core,
		path:       path,
		justOpened: true,
	}

	stream, err := coreStream.open()
	if err != nil {
		return nil, err
	}

	coreStream.EntryStorage = tcom.NewEntryStorage(stream)

	return coreStream, nil
}

func (cs *coreDirectoryStream) open() (<-chan tcom.PartialEntry, error) {
	// get the core stream
	coreCtx, coreCancel := context.WithCancel(cs.openCtx)
	coreDirChan, err := cs.core.Unixfs().Ls(coreCtx, cs.path)
	if err != nil {
		coreCancel()
		return nil, err
	}
	cs.coreCtx, cs.coreCancel = coreCtx, coreCancel

	// translate the core entries to common entries
	listChan := make(chan tcom.PartialEntry, 1) // closed by translateEntries
	go translateEntries(coreDirChan, listChan)
	return listChan, nil
}

func (cs *coreDirectoryStream) Close() error {
	if cs.coreCancel == nil {
		return tcom.ErrNotOpen // double close is considered an error
	}

	cs.coreCancel()
	cs.coreCancel = nil

	cs.err = tcom.ErrNotOpen // invalidate future operations as we're closed
	return nil
}

func (cs *coreDirectoryStream) Reset() error {
	// TODO: [d21a38b9-e723-4068-ad72-7473b91cc770] remove the disobeying rule and fix it in the FUSE layer
	// if offset == 0 && !dir.JustOpened(); reset
	// so that we don't open a stream and immediately reset it

	if cs.justOpened { // HACK: [d21a38b9-e723-4068-ad72-7473b91cc770] disobey the caller and retain our cache
		cs.justOpened = false
		return nil
	}

	if err := cs.Close(); err != nil { // invalidate the old stream
		cs.err = err
		return err
	}

	stream, err := cs.open() // get a new stream
	if err != nil {
		cs.err = err
		return err
	}
	cs.EntryStorage.Reset(stream) // reset the entry store

	cs.err = nil // clear error state, if any
	return nil
}

func errWrap(err error) <-chan transform.DirectoryEntry {
	errChan := make(chan transform.DirectoryEntry, 1)
	errChan <- &tcom.ErrorEntry{err}
	return errChan
}

func (cs *coreDirectoryStream) List(ctx context.Context, offset uint64) <-chan transform.DirectoryEntry {
	if cs.err != nil { // refuse to operate
		return errWrap(cs.err)
	}

	if cs.justOpened { // HACK: [d21a38b9-e723-4068-ad72-7473b91cc770]
		cs.justOpened = false
	}

	if cs.EntryStorage == nil {
		err := errors.New("directory not initialized")
		cs.err = err
		return errWrap(err)
	}

	return cs.EntryStorage.List(ctx, offset)
}

type dirEntryTranslator struct{ coreiface.DirEntry }

func (ce *dirEntryTranslator) Name() string { return ce.DirEntry.Name }
func (ce *dirEntryTranslator) Error() error { return ce.DirEntry.Err }

// TODO: review cancel semantics;
func translateEntries(in <-chan coreiface.DirEntry, out chan<- tcom.PartialEntry) {
	for ent := range in {
		msg := &dirEntryTranslator{DirEntry: ent}
		out <- msg
		if ent.Err != nil {
			break // exit after relaying a message that contained an error
		}
	}
	close(out)
}
