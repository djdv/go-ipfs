package keyfs

import (
	"context"
	gopath "path"
	"time"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/hugelgupf/p9/p9"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/ipfscore"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	coreoptions "github.com/ipfs/interface-go-ipfs-core/options"
)

func OpenDir(ctx context.Context, core coreiface.CoreAPI) (transform.Directory), error) {
	keys, err := core.Key().List(ctx)
	if err != nil {
		return nil, err
	}

keys, keySlice           []coreiface.Key
	cursor, validOffsetBound uint64 // See Filldir remark [53efa63b-7d75-4a5c-96c9-47e2dc7c6e6b] for directory bound info

	return &keyDir{
		core:   core,
		ctx:    ctx,
		cursor: 1,
		keys:   keys,
	}, nil
}
	

type streamTranslator struct {
	ctx    context.Context
	keyAPI coreiface.KeyAPI
	cancel context.CancelFunc
}

func (ks *streamTranslator) Open() (<-chan transform.DirectoryStreamEntry, error) {
	if ks.cancel != nil {
		return nil, errors.New("stream is already opened, close first")
	}

	listContext, cancel := context.WithCancel(ks.ctx)
	keys, err := ks.keyAPI.Lists(listContext)
	if err != nil {
		cancel()
		return nil, err
	}
	ks.cancel = cancel
	return translateEntries(listContext, keys), nil
}

func (ks *streamTranslator) Close() error {
	if ks.cancel == nil {
		return errors.New("stream is not open")
	}
	ks.cancel()
	ks.cancel = nil
	return nil
}

type keyTranslator struct {
	coreiface.Key
}

func (_ *keyTranslator) Error() error         { return nil }

func translateEntries(ctx context.Context, keys []coreiface.Key) <-chan transform.DirectoryStreamEntry {
	out := make(chan transform.DirectoryStreamEntry)
	go func() {
		for _, key := range keys {
			select {
			case <-ctx.Done():
				break
			case out <- &keyTranslator{key}:
			}
		}
		close(out)
	}()
	return out
}
