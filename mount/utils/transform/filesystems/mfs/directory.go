package mfs

import (
	"context"
	"errors"
	"fmt"

	"github.com/ipfs/go-ipfs/mount/utils/transform"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/ipfscore"
	gomfs "github.com/ipfs/go-mfs"
	uio "github.com/ipfs/go-unixfs/io"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

// OpenDir returns a Directory for the given path (as a stream of entries)
func OpenDir(ctx context.Context, mroot *gomfs.Root, path string, core coreiface.CoreAPI) (transform.Directory, error) {

	coreStream := &streamTranslator{
		ctx:   ctx,
		path:  path,
		mroot: mroot,
		core:  core,
	}

	// TODO: consider writing another stream type that handles MFS so we can drop the reliance on the core here
	return ipfscore.OpenStream(ctx, coreStream, core)
}

type streamTranslator struct {
	ctx    context.Context
	mroot  *gomfs.Root
	path   string
	core   coreiface.CoreAPI
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

	if mfsNode.Type() != gomfs.TDir {
		return nil, fmt.Errorf("%q is not a directory (type: %v)", cs.path, mfsNode.Type())
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

	unixDir, err := uio.NewDirectoryFromNode(cs.core.Dag(), ipldNode)
	if err != nil {
		return nil, err
	}

	directoryContext, cancel := context.WithCancel(cs.ctx)
	cs.cancel = cancel

	return translateEntries(directoryContext, unixDir), nil
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

func translateEntries(ctx context.Context, unixDir uio.Directory) <-chan transform.DirectoryStreamEntry {
	out := make(chan transform.DirectoryStreamEntry)
	go func() {
		for linkRes := range unixDir.EnumLinksAsync(ctx) {
			msg := &mfsListingTranslator{
				err:  linkRes.Err,
				name: linkRes.Link.Name,
				path: corepath.IpldPath(linkRes.Link.Cid),
			}
			select {
			case <-ctx.Done():
				return
			case out <- msg:
				if msg.err != nil {
					return // bail after relaying error
				}
			}
		}
		close(out)
	}()

	return out
}
