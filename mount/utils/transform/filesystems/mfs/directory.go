package mfs

import (
	"context"
	"errors"
	"fmt"

	"github.com/ipfs/go-ipfs/mount/utils/transform"
	tcom "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems"
	gomfs "github.com/ipfs/go-mfs"
	"github.com/ipfs/go-unixfs"
	uio "github.com/ipfs/go-unixfs/io"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

// TODO: make a pass on everything [AM] [hasty port]

type mfsDirectoryStream struct {
	openCtx, listCtx context.Context
	listCancel       context.CancelFunc
	mroot            *gomfs.Root
	path             string
	core             coreiface.CoreAPI
	err              error
	tcom.EntryStorage
}

// OpenDir returns a Directory for the given path (as a stream of entries)
func OpenDir(ctx context.Context, mroot *gomfs.Root, path string, core coreiface.CoreAPI) (transform.Directory, error) {

	mfsStream := &mfsDirectoryStream{
		openCtx: ctx,
		path:    path,
		mroot:   mroot,
		core:    core, // TODO: drop this for mfs/ipld native methods
	}

	stream, err := mfsStream.open()
	if err != nil {
		return nil, err
	}

	mfsStream.EntryStorage = tcom.NewEntryStorage(ctx, stream)

	return mfsStream, nil
}

// Open opens the source stream and returns a stream of translated entries
func (ms *mfsDirectoryStream) open() (<-chan tcom.PartialEntry, error) {
	mfsNode, err := gomfs.Lookup(ms.mroot, ms.path)
	if err != nil {
		return nil, err
	}

	if mfsNode.Type() != gomfs.TDir {
		return nil, fmt.Errorf("%q is not a directory (type: %v)", ms.path, mfsNode.Type())
	}

	// TODO: store the snapshot here manually; also needed for dropping core

	// NOTE:
	// We do not use the MFS directory construct here
	// as the MFS directory carries locking semantics with it internally, which cause a deadlock for us.
	// The fresh-data guarantees it provides are not necessary for SUS compliance
	// (see `readdir`'s unspecified behavior about modified contents post `opendir`)
	// and more importantly, there is no way to prepare the entry stream without maintaining a lock within the MFS directory.
	// This causes a deadlock during operation as we expect to call `Getattr` on child entries of open directories, prior to calling `closedir`
	//
	// Instead we get a snapshot of the directory as it is at the moment of `opendir`
	// and simply use that independently
	// the user may refresh the contents in a portable manner by using the standard convention (`rewinddir`)
	ipldNode, err := mfsNode.GetNode()
	if err != nil {
		return nil, err
	}

	unixDir, err := uio.NewDirectoryFromNode(ms.core.Dag(), ipldNode)
	if err != nil {
		return nil, err
	}

	listCtx, listCancel := context.WithCancel(ms.openCtx)
	unixChan := unixDir.EnumLinksAsync(listCtx)
	ms.listCtx, ms.listCancel = listCtx, listCancel

	// translate the pins to common entries (buffering the next entry between reads as well)
	listChan := make(chan tcom.PartialEntry, 1) // closed by translateEntries
	go translateEntries(listCtx, unixChan, listChan)

	return listChan, nil
}

func (ms *mfsDirectoryStream) Close() error {
	if ms.listCancel == nil {
		return tcom.ErrNotOpen // double close is considered an error
	}

	ms.listCancel()
	ms.listCancel = nil

	ms.err = tcom.ErrNotOpen // invalidate future operations as we're closed
	return nil
}

func (ms *mfsDirectoryStream) Reset() error {
	if err := ms.Close(); err != nil { // invalidate the old stream
		ms.err = err
		return err
	}

	stream, err := ms.open()
	if err != nil { // get a new stream
		ms.err = err
		return err
	}

	ms.EntryStorage.Reset(stream) // reset the entry store

	ms.err = nil // clear error state, if any
	return nil
}

func errWrap(err error) <-chan transform.DirectoryEntry {
	errChan := make(chan transform.DirectoryEntry, 1)
	errChan <- &tcom.ErrorEntry{err}
	return errChan
}

func (ms *mfsDirectoryStream) List(ctx context.Context, offset uint64) <-chan transform.DirectoryEntry {
	if ms.err != nil { // refuse to operate
		return errWrap(ms.err)
	}

	if ms.EntryStorage == nil {
		err := errors.New("directory not initialized")
		ms.err = err
		return errWrap(err)
	}
	return ms.EntryStorage.List(ctx, offset)
}

type mfsListingTranslator struct {
	name string
	err  error
}

func (me *mfsListingTranslator) Name() string { return me.name }
func (me *mfsListingTranslator) Error() error { return me.err }

func translateEntries(ctx context.Context, in <-chan unixfs.LinkResult, out chan<- tcom.PartialEntry) {
out:
	for linkRes := range in {
		msg := &mfsListingTranslator{
			err:  linkRes.Err,
			name: linkRes.Link.Name,
		}
		select {
		case out <- msg:
			if linkRes.Err != nil {
				break out // exit after relaying a message with an error
			}
		case <-ctx.Done():
			break out
		}
	}
	close(out)
}
