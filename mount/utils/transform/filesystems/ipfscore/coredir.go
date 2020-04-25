package ipfscore

import (
	"context"
	"errors"

	"github.com/ipfs/go-ipfs/mount/utils/transform"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

// OpenDir returns a Directory for the given path (as a stream of entries)
func OpenDir(ctx context.Context, path corepath.Path, core coreiface.CoreAPI) (*coreDirectoryStream, error) {
	coreStream := &streamTranslator{
		ctx:  ctx,
		path: path,
		core: core,
	}

	return OpenStream(ctx, coreStream, core)
}

type streamTranslator struct {
	ctx    context.Context
	path   corepath.Path
	core   coreiface.CoreAPI
	cancel context.CancelFunc
}

// Open opens the source stream and returns a stream of translated entries
func (cs *streamTranslator) Open() (<-chan transform.DirectoryStreamEntry, error) {
	if cs.cancel != nil {
		return nil, errors.New("already opened")
	}

	lsContext, cancel := context.WithCancel(cs.ctx)
	dirChan, err := cs.core.Unixfs().Ls(lsContext, cs.path)
	if err != nil {
		cancel()
		return nil, err
	}
	cs.cancel = cancel
	return translateEntries(lsContext, cs.path, dirChan), nil
}

func (cs *streamTranslator) Close() error {
	if cs.cancel == nil {
		return errors.New("not opened")
	}
	cs.cancel()
	cs.cancel = nil
	return nil
}

type dirEntryTranslator struct {
	coreiface.DirEntry
	parent corepath.Path
}

func (ce *dirEntryTranslator) Name() string        { return ce.DirEntry.Name }
func (ce *dirEntryTranslator) Path() corepath.Path { return corepath.Join(ce.parent, ce.DirEntry.Name) }
func (ce *dirEntryTranslator) Error() error        { return ce.DirEntry.Err }

func translateEntries(ctx context.Context, parent corepath.Path, in <-chan coreiface.DirEntry) <-chan transform.DirectoryStreamEntry {
	out := make(chan transform.DirectoryStreamEntry)
	go func() {
		for ent := range in {
			select {
			case <-ctx.Done():
				break
			case out <- &dirEntryTranslator{
				DirEntry: ent,
				parent:   parent,
			}:
			}
		}
		close(out)
	}()
	return out
}
