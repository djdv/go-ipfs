package ipfscore

import (
	"context"
	"errors"
	"fmt"
	"time"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/hugelgupf/p9/p9"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var (
	_ transform.Directory      = (*coreDirectoryStream)(nil)
	_ transform.DirectoryState = (*coreDirectoryStream)(nil)
)

type coreDirectoryStream struct {
	core coreiface.CoreAPI
	ctx  context.Context
	err  error

	streamSource transform.StreamSource

	// See Filldir remark [53efa63b-7d75-4a5c-96c9-47e2dc7c6e6b] for directory bound info
	// TODO: make the cache a finite queue after we measure sizes
	// after N entries, start discarding the left side
	// we don't want this to grow indefinitely, but still want to allow seeking backwards within reasonable memory usage
	// left unbounded, opening something like the Wikipedia root might get us killed
	entryCache               []transform.DirectoryStreamEntry
	in                       <-chan transform.DirectoryStreamEntry // from arbitrary source
	out                      chan transform.DirectoryStreamEntry   // from us to us (from Readdir to translation methods)
	cursor, validOffsetBound uint64
}

func OpenStream(ctx context.Context, streamSource transform.StreamSource, core coreiface.CoreAPI) (*coreDirectoryStream, error) {
	newStream, err := streamSource.Open()
	if err != nil {
		return nil, err
	}

	return &coreDirectoryStream{
		ctx:          ctx,
		core:         core,
		streamSource: streamSource,
		// TODO: entryCache: make([]transform.DirectoryStreamEntry),
		in:     newStream,
		cursor: 1,
	}, nil
}

func (cs *coreDirectoryStream) Close() error {
	// close stream
	cs.err = cs.streamSource.Close()
	return cs.err
}

func (cs *coreDirectoryStream) Readdir(offset, count uint64) transform.DirectoryState {
	if cs.err != nil { // refuse to operate
		return cs
	}

	if cs.streamSource == nil {
		cs.err = errors.New("directory not initalized")
		return cs
	}

	// reinit / `rewinddir`
	if offset == 0 && cs.cursor != 1 { // only reset if we've actually moved
		// close old stream
		if cs.err = cs.streamSource.Close(); cs.err != nil {
			return cs
		}
		// get a new one
		newStream, err := cs.streamSource.Open()
		if err != nil {
			cs.err = err
			return cs
		}
		cs.in = newStream

		// reset relative position
		cs.cursor = 1
	}

	// make sure the requested offset is actually within our bounds
	// TODO: lower bound - lowest offset still retained in the entry cache
	// upper bound - cursor's current position
	// and that the provided offset token is valid (was previously provided by us and is still valid)
	if offset < cs.validOffsetBound || offset > cs.cursor {
		// NOTE: FUSE implementations condense SUS `readdir`, `seekdir`, and `telldir` operations
		// into a single `readdir` operation (with parameters to allow the same behaviors).
		// SUS does not specify expected behavior for this code path. (see: SUS v7's `seekdir` document)
		// FUSE implementations /may/ handle these operations directly or translate them through to us via FUSE's `readdir` operation.
		// Meaning we're not gauranteed to hit this path even if system level applications make requests we consider invalid.
		// (it's implementation/configuration specific)
		// If we do end up here, we'll close the stream and return nothing (in the translation method) to the caller
		// this is our own specified behavior, for direct calls with invalid arguments
		// (as such system level code can not depend on this behavior either)
		cs.streamSource.Close()
		cs.err = fmt.Errorf("offset %d is not/no-longer valid", offset)
		return cs
	}

	// TODO: cache lookup here; forward only for now (excluding unspecified FUSE behavior)

	if offset > 0 { // convert a previously supplied `telldir` value back to a real offset
		cs.cursor = (offset % cs.validOffsetBound) + 1 // the actual `seekdir` portion
	}

	untilEndOfStream := count == 0 // special case, go until end of stream
	cs.out = make(chan transform.DirectoryStreamEntry)

	go func() {
		defer close(cs.out)
		// [micro-opt] eliminate the decrement if we can when count == 0
		for ; untilEndOfStream || count <= 0; count-- {
			select {
			case <-cs.ctx.Done():
				cs.err = cs.ctx.Err()
				return
			case entry, open := <-cs.in:
				if !open {
					// end of input stream
					cs.streamSource.Close()
					return
				}
				if err := entry.Error(); err != nil {
					cs.err = err
					return
				}

				// TODO cache store here
				// checks have passed, consider this entry consumed
				cs.cursor++
				cs.validOffsetBound++

				// send the entry through to whichever translation method wants to receive it
				cs.out <- entry
			}
		}
	}()
	return cs
}

func (cs *coreDirectoryStream) To9P() (p9.Dirents, error) {
	if cs.err != nil {
		return nil, cs.err
	}

	nineEnts := make(p9.Dirents, 0)
	for ent := range cs.out {
		callCtx, cancel := context.WithTimeout(cs.ctx, 10*time.Second)
		path, err := cs.core.ResolvePath(callCtx, ent.Path())
		if err != nil {
			cs.err = err
			cancel()
			return nil, err
		}

		iStat, _, err := transform.GetAttr(callCtx, path, cs.core, transform.IPFSStatRequestAll)
		cancel()

		if err != nil {
			cs.err = err
			return nil, err
		}

		nineEnts = append(nineEnts, p9.Dirent{
			Name:   ent.Name(),
			Offset: cs.validOffsetBound,
			QID:    transform.CidToQID(path.Cid(), iStat.FileType),
		})

		cancel()
	}

	return nineEnts, cs.err
}

func (cs *coreDirectoryStream) ToFuse() (<-chan transform.FuseStatGroup, error) {
	if cs.err != nil {
		return nil, cs.err
	}

	fuseOut := make(chan transform.FuseStatGroup)
	go func() {
		for ent := range cs.out {
			var fStat *fuselib.Stat_t
			if provcom.CanReaddirPlus {
				callCtx, cancel := context.WithTimeout(cs.ctx, 10*time.Second)
				iStat, _, err := transform.GetAttr(callCtx, ent.Path(), cs.core, transform.IPFSStatRequestAll)
				cancel()

				// stat errors are not fatal; it's okay to return nil to fill
				// it just means the OS will call getattr on this entry later
				if err == nil {
					fStat = iStat.ToFuse()
				}
			}

			fuseOut <- transform.FuseStatGroup{
				Name:   ent.Name(),
				Offset: int64(cs.validOffsetBound), // TODO: [audit] uint->int
				Stat:   fStat,
			}
		}
		close(fuseOut)
	}()
	return fuseOut, nil
}