package pinfs

import (
	"context"
	"errors"
	gopath "path"

	"github.com/ipfs/go-ipfs/mount/utils/transform"
	tcom "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	coreoptions "github.com/ipfs/interface-go-ipfs-core/options"
)

// TODO: make a pass on everything [AM] [hasty port]

type pinDirectoryStream struct {
	openCtx, coreCtx context.Context
	coreCancel       context.CancelFunc
	pinAPI           coreiface.PinAPI
	err              error
	tcom.EntryStorage
}

// OpenDir returns a Directory containing the node's pins (as a stream of entries)
func OpenDir(ctx context.Context, core coreiface.CoreAPI) (transform.Directory, error) {
	pinStream := &pinDirectoryStream{
		openCtx: ctx,
		pinAPI:  core.Pin(),
	}

	stream, err := pinStream.open()
	if err != nil {
		return nil, err
	}

	pinStream.EntryStorage = tcom.NewEntryStorage(ctx, stream)

	return pinStream, nil
}

func (ps *pinDirectoryStream) open() (<-chan tcom.PartialEntry, error) {
	// get the pin stream
	coreCtx, coreCancel := context.WithCancel(ps.openCtx)
	pinChan, err := ps.pinAPI.Ls(coreCtx, coreoptions.Pin.Ls.Recursive())
	if err != nil {
		coreCancel()
		return nil, err
	}
	ps.coreCtx, ps.coreCancel = coreCtx, coreCancel

	// translate the pins to common entries (buffering the next entry between reads as well)
	listChan := make(chan tcom.PartialEntry, 1) // closed by translateEntries
	go translateEntries(coreCtx, pinChan, listChan)
	return listChan, nil
}

func (ps *pinDirectoryStream) Close() error {
	if ps.coreCancel == nil {
		return tcom.ErrNotOpen // double close is considered an error
	}

	ps.coreCancel()
	ps.coreCancel = nil

	ps.err = tcom.ErrNotOpen // invalidate future operations as we're closed
	return nil
}

func (ps *pinDirectoryStream) Reset() error {
	if err := ps.Close(); err != nil { // invalidate the old stream
		ps.err = err
		return err
	}

	stream, err := ps.open()
	if err != nil { // get a new stream
		ps.err = err
		return err
	}

	ps.EntryStorage.Reset(stream) // reset the entry store

	ps.err = nil // clear error state, if any
	return nil
}

func errWrap(err error) <-chan transform.DirectoryEntry {
	errChan := make(chan transform.DirectoryEntry, 1)
	errChan <- &tcom.ErrorEntry{err}
	return errChan
}

func (ps *pinDirectoryStream) List(ctx context.Context, offset uint64) <-chan transform.DirectoryEntry {
	if ps.err != nil { // refuse to operate
		return errWrap(ps.err)
	}

	if ps.EntryStorage == nil {
		err := errors.New("directory not initialized")
		ps.err = err
		return errWrap(err)
	}
	return ps.EntryStorage.List(ctx, offset)
}

type pinEntryTranslator struct{ coreiface.Pin }

func (pe *pinEntryTranslator) Name() string { return gopath.Base(pe.Path().String()) }
func (pe *pinEntryTranslator) Error() error { return pe.Err() }

// TODO: review cancel semantics;
func translateEntries(ctx context.Context, pins <-chan coreiface.Pin, out chan<- tcom.PartialEntry) {
out:
	for pin := range pins {
		msg := &pinEntryTranslator{Pin: pin}

		select {
		// translate the entry and try to send it
		case out <- msg:
			if pin.Err() != nil {
				break out // exit after relaying a message with an error
			}

		// or bail if we're canceled
		case <-ctx.Done():
			break out
		}
	}
	close(out)
}
