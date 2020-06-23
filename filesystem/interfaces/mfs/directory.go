package mfs

import (
	"context"
	"fmt"

	"github.com/ipfs/go-ipfs/filesystem"
	tcom "github.com/ipfs/go-ipfs/filesystem/interfaces"
	gomfs "github.com/ipfs/go-mfs"
)

// TODO: make a pass on everything [AM] [hasty port]

type mfsDirectoryStream struct {
	mroot *gomfs.Root
	path  string
}

// OpenDirectory returns a Directory for the given path (as a stream of entries)
func (mi *mfsInterface) OpenDirectory(path string) (filesystem.Directory, error) {
	mfsStream := &mfsDirectoryStream{
		path:  path,
		mroot: mi.mroot,
	}

	return tcom.PartialEntryUpgrade(
		tcom.NewStreamBase(mi.ctx, mfsStream))
}

// SendTo receives a channel with which we will send entries to, until the context is caneled, or the end of stream is reached
func (ms *mfsDirectoryStream) SendTo(ctx context.Context, receiver chan<- tcom.PartialEntry) error {
	mfsNode, err := gomfs.Lookup(ms.mroot, ms.path)
	if err != nil {
		close(receiver)
		return err
	}

	if mfsNode.Type() != gomfs.TDir {
		close(receiver)
		return fmt.Errorf("%q is not a directory (type: %v)", ms.path, mfsNode.Type())
	}

	mfsDir := mfsNode.(*gomfs.Directory)

	snapshot, err := mfsDir.ListNames(ctx)
	if err != nil {
		close(receiver)
		return err
	}

	// start sending translated entries to the receiver
	go translateEntries(ctx, snapshot, receiver)

	return nil
}

type mfsListingTranslator string

func (me mfsListingTranslator) Name() string { return string(me) }
func (me mfsListingTranslator) Error() error { return nil }

func translateEntries(ctx context.Context, in []string, out chan<- tcom.PartialEntry) {
out:
	for _, name := range in {
		select {
		case out <- mfsListingTranslator(name):
		case <-ctx.Done():
			break out
		}
	}
	close(out)
}
