package keyfs

import (
	"context"
	"errors"

	"github.com/ipfs/go-ipfs/mount/utils/transform"
	tcom "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

// TODO: make a pass on everything [AM] [hasty port]

type keyDirectoryStream struct {
	openCtx, coreCtx context.Context
	coreCancel       context.CancelFunc
	keyAPI           coreiface.KeyAPI
	err              error
	tcom.EntryStorage
}

func OpenDir(ctx context.Context, core coreiface.CoreAPI) (transform.Directory, error) {
	keyStream := &keyDirectoryStream{
		openCtx: ctx,
		keyAPI:  core.Key(),
	}

	stream, err := keyStream.open()
	if err != nil {
		return nil, err
	}

	keyStream.EntryStorage = tcom.NewEntryStorage(ctx, stream)

	return keyStream, nil
}

func (ks *keyDirectoryStream) open() (<-chan tcom.PartialEntry, error) {
	coreCtx, coreCancel := context.WithCancel(ks.openCtx)

	// prepare the keys
	keys, err := ks.keyAPI.List(coreCtx)
	if err != nil {
		coreCancel()
		return nil, err
	}
	ks.coreCtx, ks.coreCancel = coreCtx, coreCancel

	// translate the pins to common entries (buffering the next entry between reads as well)
	listChan := make(chan tcom.PartialEntry, 1)
	go translateEntries(coreCtx, keys, listChan)
	return listChan, nil
}

func (ks *keyDirectoryStream) Close() error {
	if ks.coreCancel == nil {
		return tcom.ErrNotOpen // double close is considered an error
	}

	ks.coreCancel()
	ks.coreCancel = nil

	ks.err = tcom.ErrNotOpen // invalidate future operations as we're closed
	return nil
}

func (ks *keyDirectoryStream) Reset() error {
	if err := ks.Close(); err != nil { // invalidate the old stream
		ks.err = err
		return err
	}

	stream, err := ks.open()
	if err != nil { // get a new stream
		ks.err = err
		return err
	}

	ks.EntryStorage.Reset(stream) // reset the entry store

	ks.err = nil // clear error state, if any
	return nil
}

func errWrap(err error) <-chan transform.DirectoryEntry {
	errChan := make(chan transform.DirectoryEntry, 1)
	errChan <- &tcom.ErrorEntry{err}
	return errChan
}

func (ks *keyDirectoryStream) List(ctx context.Context, offset uint64) <-chan transform.DirectoryEntry {
	if ks.err != nil { // refuse to operate
		return errWrap(ks.err)
	}

	if ks.EntryStorage == nil {
		err := errors.New("directory not initialized")
		ks.err = err
		return errWrap(err)
	}
	return ks.EntryStorage.List(ctx, offset)
}

type keyTranslator struct{ coreiface.Key }

func (ke *keyTranslator) Name() string { return ke.Key.Name() }
func (_ *keyTranslator) Error() error  { return nil }

func translateEntries(ctx context.Context, keys []coreiface.Key, out chan<- tcom.PartialEntry) {
out:
	for _, key := range keys {
		select {
		case <-ctx.Done():
			break out
		case out <- &keyTranslator{key}:
		}
	}
	close(out)
}
