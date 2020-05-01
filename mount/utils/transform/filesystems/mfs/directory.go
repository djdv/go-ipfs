package mfs

import (
	"context"
	"errors"
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/ipfscore"
	gomfs "github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

// OpenDir returns a Directory for the given path (as a stream of entries)
func OpenDir(ctx context.Context, mroot *gomfs.Root, path string, core coreiface.CoreAPI) (transform.Directory, error) {

	coreStream := &streamTranslator{
		ctx:   ctx,
		path:  path,
		mroot: mroot,
	}

	// TODO: consider writing another stream type that handles MFS so we can drop the reliance on the core here
	return ipfscore.OpenStream(ctx, coreStream, core)
}

type streamTranslator struct {
	ctx    context.Context
	mroot  *gomfs.Root
	path   string
	cancel context.CancelFunc
}

// Open opens the source stream and returns a stream of translated entries
func (cs *streamTranslator) Open() (<-chan transform.DirectoryStreamEntry, error) {
	if cs.cancel != nil {
		return nil, errors.New("already opened")
	}

	mfsNode, err := gomfs.Lookup(cs.mroot, cs.path)
	if err != nil {
		return nil, err
	}

	mfsDir, ok := mfsNode.(*gomfs.Directory)
	if !ok {
		return nil, fmt.Errorf("%q is not a directory (type: %v)", cs.path, mfsNode.Type())
	}

	directoryContext, cancel := context.WithCancel(cs.ctx)

	cs.cancel = cancel
	return translateEntries(directoryContext, mfsDir), nil
}

func (cs *streamTranslator) Close() error {
	if cs.cancel == nil {
		return errors.New("not opened")
	}
	cs.cancel()
	cs.cancel = nil
	return nil
}

type mfsListingTranslator struct {
	name string
	path corepath.Path
	err  error
}

func (me *mfsListingTranslator) Name() string        { return me.name }
func (me *mfsListingTranslator) Path() corepath.Path { return me.path }
func (me *mfsListingTranslator) Error() error        { return me.err }

func translateEntries(ctx context.Context, mfsDir *gomfs.Directory) <-chan transform.DirectoryStreamEntry {
	out := make(chan transform.DirectoryStreamEntry)

	go func() {
		mfsDir.ForEachEntry(ctx, func(listing gomfs.NodeListing) error {
			msg := &mfsListingTranslator{name: listing.Name}

			cid, err := cid.Decode(listing.Hash)
			if err != nil {
				msg.err = err
			} else {
				msg.path = corepath.IpldPath(cid)
			}

			select {
			case <-ctx.Done():
				return ctx.Err() // bail
			case out <- msg: // relay
				if msg.err != nil { // we were not canceled but did error, so bail after relaying the error
					return msg.err
				}
			}

			return nil
		})
		close(out)
	}()

	return out
}
