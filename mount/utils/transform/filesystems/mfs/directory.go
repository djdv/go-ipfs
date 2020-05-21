package mfs

import (
	"context"
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
	mroot *gomfs.Root
	path  string
	core  coreiface.CoreAPI // TODO: drop this for mfs/ipld native methods
}

// OpenDir returns a Directory for the given path (as a stream of entries)
func OpenDir(ctx context.Context, mroot *gomfs.Root, path string, core coreiface.CoreAPI) (transform.Directory, error) {
	mfsStream := &mfsDirectoryStream{
		path:  path,
		mroot: mroot,
		core:  core,
	}
	return tcom.PartialEntryUpgrade(
		tcom.NewCoreStreamBase(ctx, mfsStream))
}

// Open opens the source stream and returns a stream of translated entries
func (ms *mfsDirectoryStream) SendTo(ctx context.Context, receiver chan<- tcom.PartialEntry) error {
	mfsNode, err := gomfs.Lookup(ms.mroot, ms.path)
	if err != nil {
		return err
	}

	if mfsNode.Type() != gomfs.TDir {
		return fmt.Errorf("%q is not a directory (type: %v)", ms.path, mfsNode.Type())
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
		return err
	}

	unixDir, err := uio.NewDirectoryFromNode(ms.core.Dag(), ipldNode)
	if err != nil {
		return err
	}

	unixChan := unixDir.EnumLinksAsync(ctx)

	// start sending translated entries to the receiver
	go translateEntries(ctx, unixChan, receiver)

	return nil
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
