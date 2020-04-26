package keyfs

import (
	"context"
	"errors"

	"github.com/ipfs/go-ipfs/mount/utils/transform"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/ipfscore"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

func OpenDir(ctx context.Context, keyAPI coreiface.KeyAPI) (transform.Directory, error) {

	keyStream := &streamTranslator{
		ctx:    ctx,
		keyAPI: keyAPI,
	}

	return ipfscore.OpenStream(ctx, keyStream)
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
	keys, err := ks.keyAPI.List(listContext)
	if err != nil {
		cancel()
		return nil, err
	}
	ks.keys = keys
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

func (_ *keyTranslator) Error() error { return nil }

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
