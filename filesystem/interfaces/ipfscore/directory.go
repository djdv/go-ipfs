package ipfscore

import (
	"context"

	transform "github.com/ipfs/go-ipfs/filesystem"
	tcom "github.com/ipfs/go-ipfs/filesystem/interfaces"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

type coreDirectoryStream struct {
	core coreiface.CoreAPI // used during stream source construction
	path corepath.Path     // the streams source location; used to (re)construct the stream in `Open`/`Reset`
}

// OpenDirectory returns a Directory for the given path (as a stream of entries)
func (ci *coreInterface) OpenDirectory(path string) (transform.Directory, error) {
	coreStream := &coreDirectoryStream{
		core: ci.core,
		path: ci.joinRoot(path),
	}

	return tcom.PartialEntryUpgrade(
		tcom.NewStreamBase(ci.ctx, coreStream))
}

// SendTo receives a channel with which we will send entries to, until the context is caneled, or the end of stream is reached
func (cs *coreDirectoryStream) SendTo(ctx context.Context, receiver chan<- tcom.PartialEntry) error {
	coreDirChan, err := cs.core.Unixfs().Ls(ctx, cs.path)
	if err != nil {
		return err
	}

	// start sending translated entries to the receiver
	go translateEntries(coreDirChan, receiver)

	return nil
}

type dirEntryTranslator struct{ coreiface.DirEntry }

func (ce *dirEntryTranslator) Name() string { return ce.DirEntry.Name }
func (ce *dirEntryTranslator) Error() error { return ce.DirEntry.Err }

// TODO: review cancel semantics
// `in` should be closed if the Ls context is canceled, so we shouldn't need to be aware of the `ctx` here
// needs cancel tests though
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
