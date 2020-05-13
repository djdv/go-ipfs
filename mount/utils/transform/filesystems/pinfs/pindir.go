package pinfs

import (
	"context"
	"errors"
	gopath "path"

	"github.com/ipfs/go-ipfs/mount/utils/transform"
	coreipfs "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/ipfscore"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	coreoptions "github.com/ipfs/interface-go-ipfs-core/options"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

// OpenDir returns a Directory containing the node's pins (as a stream of entries)
func OpenDir(ctx context.Context, core coreiface.CoreAPI) (transform.Directory, error) {
	pinStream := &streamTranslator{
		ctx:    ctx,
		pinAPI: core.Pin(),
	}

	return coreipfs.OpenStream(ctx, pinStream, core)
}

type streamTranslator struct {
	ctx    context.Context
	pinAPI coreiface.PinAPI
	cancel context.CancelFunc
}

func (ps *streamTranslator) Open() (<-chan transform.DirectoryStreamEntry, error) {
	if ps.cancel != nil {
		return nil, errors.New("stream is already opened, close first")
	}

	lsContext, cancel := context.WithCancel(ps.ctx)
	pins, err := ps.pinAPI.Ls(lsContext, coreoptions.Pin.Ls.Recursive())
	if err != nil {
		cancel()
		return nil, err
	}
	ps.cancel = cancel
	return translateEntries(lsContext, pins), nil
}

func (ps *streamTranslator) Close() error {
	if ps.cancel == nil {
		return errors.New("stream is not open")
	}
	ps.cancel()
	ps.cancel = nil
	return nil
}

type pinEntryTranslator struct {
	coreiface.Pin
}

// this is silly but we need the signature to match; ResolvedPath != Path
func (pe *pinEntryTranslator) Path() corepath.Path { return pe.Pin.Path() }
func (pe *pinEntryTranslator) Name() string        { return gopath.Base(pe.Path().String()) }
func (_ *pinEntryTranslator) Error() error         { return nil }

func translateEntries(ctx context.Context, pins <-chan coreiface.Pin) <-chan transform.DirectoryStreamEntry {
	out := make(chan transform.DirectoryStreamEntry)
	go func() {
		for pin := range pins {
			select {
			case <-ctx.Done():
				break
			case out <- &pinEntryTranslator{pin}:
			}
		}
		close(out)
	}()
	return out
}
