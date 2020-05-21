package keyfs

import (
	"context"

	"github.com/ipfs/go-ipfs/mount/utils/transform"
	tcom "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

// TODO: make a pass on everything [AM] [hasty port]

type keyDirectoryStream struct {
	keyAPI coreiface.KeyAPI
}

func OpenDir(ctx context.Context, core coreiface.CoreAPI) (transform.Directory, error) {
	return tcom.PartialEntryUpgrade(
		tcom.NewCoreStreamBase(ctx, &keyDirectoryStream{keyAPI: core.Key()}))
}

func (ks *keyDirectoryStream) SendTo(ctx context.Context, receiver chan<- tcom.PartialEntry) error {
	// prepare the keys
	keys, err := ks.keyAPI.List(ctx)
	if err != nil {
		return err
	}

	// start sending translated entries to the receiver
	go translateEntries(ctx, keys, receiver)
	return nil
}

type keyTranslator struct{ coreiface.Key }

func (ke *keyTranslator) Name() string { return ke.Key.Name() }
func (_ *keyTranslator) Error() error  { return nil }

func translateEntries(ctx context.Context, keys []coreiface.Key, out chan<- tcom.PartialEntry) {
out:
	for _, key := range keys {
		select {
		case <-ctx.Done():
			break out
		case out <- &keyTranslator{key}:
		}
	}
	close(out)
}
