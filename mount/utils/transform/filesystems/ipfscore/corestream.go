package ipfscore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/hugelgupf/p9/p9"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var (
	_ transform.Directory       = (*coreDirectoryStream)(nil)
	_ transform.DirectoryStream = (*coreDirectoryStream)(nil)
	_ transform.DirectoryState  = (*coreDirectoryStream)(nil)
)

type offsetStreamEntry interface {
	transform.DirectoryStreamEntry
	Offset() uint64
}

type offsetEntry struct {
	transform.DirectoryStreamEntry
	offset uint64
}

func (oe *offsetEntry) Offset() uint64 { return oe.offset }

type coreDirectoryStream struct {
	core             coreiface.CoreAPI
	openCtx, readCtx context.Context
	err              error

	streamSource transform.StreamSource

	// See Filldir remark [53efa63b-7d75-4a5c-96c9-47e2dc7c6e6b] for directory bound info
	entryCache []*offsetEntry
	in         <-chan transform.DirectoryStreamEntry // from stream source
	out        chan offsetStreamEntry                // proccessed and sent to translation methods
	wg         sync.WaitGroup                        // prevent caller from calling readdir again before calling a translation method (which must call .Done)

	dontreset     bool // dumb FUSE hacks
	dontTranslate bool // dumb self hack
	// ^ this is to prevent the sequence `state := readdir(); state.translate(); state.translate()` which would mess up the wait group
	// this needs to be refactored out properly instead of hacked away

	cursor, upperBound uint64
}

func OpenStream(ctx context.Context, streamSource transform.StreamSource, core coreiface.CoreAPI) (*coreDirectoryStream, error) {
	newStream, err := streamSource.Open()
	if err != nil {
		return nil, err
	}

	return &coreDirectoryStream{
		openCtx:      ctx,
		core:         core,
		streamSource: streamSource,
		in:           newStream,
		entryCache:   make([]*offsetEntry, 0),
	}, nil
}

func (cs *coreDirectoryStream) Close() error {
	cs.err = cs.streamSource.Close()
	return cs.err
}

func (cs *coreDirectoryStream) DontReset() {
	cs.dontreset = true
}

func (cs *coreDirectoryStream) Readdir(ctx context.Context, offset uint64) transform.DirectoryState {
	if cs.err != nil { // refuse to operate
		return cs
	}

	// (unless we encounter an error) a call to readdir will force subsequent calls of readdir to wait
	// until a the out channel is closed (below) and a translation method has been called (which itself calls .Done())
	cs.wg.Wait()
	cs.wg.Add(2)
	cs.dontTranslate = false

	cs.readCtx = ctx

	if cs.dontreset { // unset special request flag
		defer func() { cs.dontreset = false }()
	}

	if cs.streamSource == nil {
		cs.err = errors.New("directory not initialized")
		cs.wg.Done()
		return cs
	}

	// `rewinddir` is simulated via offset 0
	if offset == 0 {
		// we only reset if a read was performed prior (unless we're asked not to; allowing for a simulated `seekdir(0)`)
		if !cs.dontreset && cs.cursor != 0 {
			cs.entryCache = make([]*offsetEntry, 0)              // invalidate cache
			if cs.err = cs.streamSource.Close(); cs.err != nil { // close old stream
				cs.wg.Add(-2)
				return cs
			}

			newStream, err := cs.streamSource.Open() // get a new one
			if err != nil {
				cs.err = err
				cs.wg.Add(-2)
				return cs
			}
			cs.in = newStream

			// invalidate the final returned offset before the stream was reset
			// the hypothetical entry it pointed to, will never be (considered) valid
			// and the head of the stream will be rebased on this new value (final value +1)
			cs.upperBound++
		}
		cs.cursor = 0 // always reset the cursor
	} else { // otherwise we treat the offset as would be done in `seekdir(offset)`
		// NOTE:
		// while SUSv7 does not specify behavior for `seekdir` values that were not returned from `telldir`
		// our implementation treats them as invalid and returns nothing
		// offset values are unique per stream instance, and become invalid when the stream is reset
		// as such, we validate these offset bounds here

		// lower bound - offset must not be lower than the first cached entry's absolute offset
		// upper bound - steams current head, as incremented by each read from the stream
		lowerBound := cs.upperBound - uint64(len(cs.entryCache))
		if offset < lowerBound || offset > cs.upperBound {
			cs.err = fmt.Errorf("offset %d is not/no-longer valid", offset)
			cs.wg.Add(-2)
			return cs
		}

		// we checked above that the offset value is within our accepted range
		// now we need to do the actual conversion from the absolute `telldir` value
		// (a value that only increases, based on the entries read)
		// back to a relative offset to use as would be done in `seekdir`
		// (an index value within the range of cached entries or 1 beyond it)
		relativeOffset := offset % cs.upperBound
		if relativeOffset == 0 {
			// if the value wrapped; offset is pointing at the head of the stream
			cs.cursor = offset
		} else {
			// otherwise it's pointing at an entry within the cache
			// +1 so as to include the full range of the cache slice
			// (i.e. don't exclude the final element in the slice)
			cs.cursor = relativeOffset % uint64(len(cs.entryCache)+1)
		}
	}

	// TODO: [audit/refactor] everything asynchronous going on in this file needs to be looked at
	cs.out = make(chan offsetStreamEntry)
	go func() {
		cs.streamFromCache()
		cs.streamFromInput()
		close(cs.out)
		cs.wg.Done()
	}()
	return cs
}

func (cs *coreDirectoryStream) streamFromCache() {
	if cs.cursor < uint64(len(cs.entryCache)) { // if cursor is within cache range, pull entires from it
		for _, ent := range cs.entryCache[cs.cursor:] {
			cs.cursor++ // and entry was read, advance the cursor
			select {
			case <-cs.openCtx.Done():
				cs.err = cs.openCtx.Err() // directory was closed; invalidate operations
				return
			case <-cs.readCtx.Done(): // read was canceled; just exit
				return
			case cs.out <- ent: // entry was relayed to translation method
			}
		}
	}
}

func (cs *coreDirectoryStream) streamFromInput() {
	for {
		select {
		case <-cs.openCtx.Done(): // directory was closed; invalidate operations
			cs.err = cs.openCtx.Err()
			return
		case <-cs.readCtx.Done(): // read was canceled; just exit
			return
		case entry, open := <-cs.in:
			if !open { // end of input stream; exit
				return
			}
			if err := entry.Error(); err != nil {
				cs.err = err // fail permanently
				return
			}

			// stream was read, advance the absolute upper bound
			cs.upperBound++

			// create an offset entry for the value
			msg := &offsetEntry{
				DirectoryStreamEntry: entry,
				offset:               cs.upperBound,
			}
			// cache entry for `seekdir` calls
			cs.entryCache = append(cs.entryCache, msg)

			// entry was read, advance the cursor
			cs.cursor++

			// relay entry to whichever translation method wants to receive it
			// or bail if we're canceled before the receiver picks up
			select {
			case <-cs.openCtx.Done():
				cs.err = cs.openCtx.Err()
				return
			case <-cs.readCtx.Done():
				return
			case cs.out <- msg:
			}
		}
	}
}

func (cs *coreDirectoryStream) To9P(count uint32) (p9.Dirents, error) {
	if cs.err != nil || cs.dontTranslate {
		return nil, cs.err
	}
	cs.dontTranslate = true

	// from 9p; I mangled it though; change this to a stream output?
	defer cs.wg.Done() // block readdir calls until we exit
	nineEnts := make(p9.Dirents, 0, count)
	for len(nineEnts) < int(count) {
		select {
		case <-cs.openCtx.Done():
			cs.err = cs.openCtx.Err()
			return nineEnts, cs.err
		case <-cs.readCtx.Done():
			return nineEnts, cs.readCtx.Err()
		case ent, ok := <-cs.out:
			if !ok {
				return nineEnts, nil
			}

			callCtx, cancel := context.WithTimeout(cs.openCtx, 10*time.Second)
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
				Offset: ent.Offset(),
				QID:    transform.CidToQID(path.Cid(), iStat.FileType),
			})

			cancel()
		}
	}

	return nineEnts, cs.err
}

func (cs *coreDirectoryStream) ToFuse() (<-chan transform.FuseStatGroup, error) {
	if cs.err != nil || cs.dontTranslate {
		return nil, cs.err
	}
	cs.dontTranslate = true

	fuseOut := make(chan transform.FuseStatGroup)
	go func() {
		defer cs.wg.Done() // block readdir calls until we exit

		for ent := range cs.out { // consume entries provided by readdir's goroutine
			var fStat *fuselib.Stat_t
			if provcom.CanReaddirPlus {
				callCtx, cancel := context.WithTimeout(cs.openCtx, 4*time.Second)
				iStat, _, err := transform.GetAttr(callCtx, ent.Path(), cs.core, transform.IPFSStatRequestAll)
				cancel()

				// stat errors are not fatal; it's okay to return nil to fill
				// it just means the OS will call getattr on this entry later
				if err == nil {
					fStat = iStat.ToFuse()
				}
			}

			msg := transform.FuseStatGroup{
				Name:   ent.Name(),
				Offset: int64(ent.Offset()), // TODO: [audit] uint->int
				Stat:   fStat,
			}

			select {
			case <-cs.openCtx.Done():
				cs.err = cs.openCtx.Err() // directory was closed; invalidate operations
				return
			case <-cs.readCtx.Done(): // read was canceled; just exit
				return
			case fuseOut <- msg: // entry was relayed to caller
			}
		}
		close(fuseOut)
	}()
	return fuseOut, nil
}

func (cs *coreDirectoryStream) ToGo() ([]os.FileInfo, error) {
	return nil, errors.New("not updated yet")
	gc, err := cs.ToGoC(nil)
	if err != nil {
		return nil, err
	}

	goEnts := make([]os.FileInfo, 0)
	for ent := range gc {
		goEnts = append(goEnts, ent)
	}

	return goEnts, cs.err
}

func (cs *coreDirectoryStream) ToGoC(goChan chan os.FileInfo) (<-chan os.FileInfo, error) {
	return nil, errors.New("not updated yet")
	if cs.err != nil {
		return nil, cs.err
	}

	if goChan == nil {
		goChan = make(chan os.FileInfo)
	}
	defer close(goChan)

	for ent := range cs.out {
		callCtx, cancel := context.WithTimeout(cs.openCtx, 10*time.Second)
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

		goChan <- iStat.ToGo(ent.Name())
		cancel()
	}

	return goChan, cs.err
}
