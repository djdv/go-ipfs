package ipfscore

import (
	"context"
	"time"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/hugelgupf/p9/p9"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

var (
	_ transform.Directory      = (*coreDir)(nil)
	_ transform.DirectoryState = (*coreDir)(nil)
)

type (
	coreEntry struct {
		coreiface.DirEntry
		offset uint64
	}
	entryChan <-chan coreiface.DirEntry
	exitChan  chan coreEntry
)

// TODO: [async safe]
// ToFuse should lock until it's done with its goroutine
type coreDir struct {
	core   coreiface.CoreAPI
	ctx    context.Context
	cancel context.CancelFunc
	err    error

	path                     corepath.Path
	entryChan                entryChan
	exitChan                 exitChan
	cursor, validOffsetBound uint64 // See Filldir remark [53efa63b-7d75-4a5c-96c9-47e2dc7c6e6b] for directory bound info
}

func (cd *coreDir) To9P() (p9.Dirents, error) {
	if cd.err != nil {
		return nil, cd.err
	}

	nineEnts := make(p9.Dirents, 0)
	for coreEntry := range cd.exitChan {
		// convert from core wrapper -> 9P
		nineEnt := transform.CoreDirEntryTo9Dirent(coreEntry.DirEntry)
		nineEnt.Offset = coreEntry.offset

		nineEnts = append(nineEnts, nineEnt)
	}

	return nineEnts, cd.err
}

func (cd *coreDir) ToFuse() (<-chan transform.FuseStatGroup, error) {
	if cd.err != nil {
		return nil, cd.err
	}

	dirChan := make(chan transform.FuseStatGroup)

	go func() {
		defer close(dirChan)
		for coreEntry := range cd.exitChan {

			var fStat *fuselib.Stat_t
			if provcom.CanReaddirPlus {
				callCtx, cancel := context.WithTimeout(cd.ctx, 10*time.Second)

				subPath := corepath.Join(cd.path, coreEntry.DirEntry.Name)
				iStat, _, err := GetAttr(callCtx, subPath, cd.core, transform.IPFSStatRequestAll)
				cancel()

				// stat errors are not fatal; it's okay to return nil to fill
				// it just means the OS will call getattr on this entry later
				if err == nil {
					fStat = iStat.ToFuse()
				}
			}

			dirChan <- transform.FuseStatGroup{
				Name:   coreEntry.Name,
				Offset: int64(coreEntry.offset),
				Stat:   fStat,
			}
		}
	}()
	return dirChan, cd.err
}

func (cd *coreDir) Readdir(offset, count uint64) transform.DirectoryState {
	if cd.err != nil { // refuse to operate
		return cd
	}

	// reinit // rewinddir
	if offset == 0 && cd.cursor != 1 { // only reset if we've actually moved
		if cd.cancel != nil { // close previous request (if any)
			cd.cancel()
		}
		close(cd.exitChan)

		operationContext, cancel := context.WithCancel(cd.ctx)
		cd.cancel = cancel

		dirChan, err := cd.core.Unixfs().Ls(operationContext, cd.path)
		if err != nil {
			cd.err = err
			cancel()
			return cd
		}

		cd.entryChan = dirChan
		cd.exitChan = make(exitChan)
		cd.cursor = 1
	}

	if offset < cd.validOffsetBound || offset > cd.cursor {
		// return NULL dirent to reader
		close(cd.exitChan)
		return cd
	}

	// TODO: we need to cache entries received for seekdir to work properly
	// for now we rely on FUSE to handle it or callers to go forward only

	untilEndOfStream := count == 0 // special case, go forever

	go func() {
		defer close(cd.exitChan)
		// [micro-opt] eliminate the decrement if we can when count == 0
		for ; untilEndOfStream || count <= 0; count-- {
			select {
			case <-cd.ctx.Done():
				cd.err = cd.ctx.Err()
				return
			case entry, open := <-cd.entryChan:
				if !open {
					// streams empty
					return
				}
				if entry.Err != nil {
					cd.err = entry.Err
					return
				}

				// send it to whichever translation method wants to receive it
				cd.exitChan <- coreEntry{DirEntry: entry, offset: cd.cursor}
				cd.cursor++
				cd.validOffsetBound++

			case <-cd.ctx.Done():
				cd.err = cd.ctx.Err()
				return
			}
		}
	}()
	return cd
}

func OpenDir(ctx context.Context, ns mountinter.Namespace, path string, core coreiface.CoreAPI) (*coreDir, error) {
	// func OpenDir(ctx context.Context, path corepath.Path, core coreiface.CoreAPI) (*coreDir, error) {

	fullPath, err := joinRoot(ns, path)
	if err != nil {
		return nil, err
	}

	operationContext, cancel := context.WithCancel(ctx)

	dirChan, err := core.Unixfs().Ls(operationContext, fullPath)
	if err != nil {
		cancel()
		return nil, err
	}

	return &coreDir{
		core:      core,
		ctx:       ctx,
		cursor:    1,
		entryChan: dirChan,
		exitChan:  make(exitChan),
		cancel:    cancel,
		path:      fullPath,
	}, nil
}

func (cd *coreDir) Close() error {
	if cd.cancel != nil {
		cd.cancel()
	}
	close(cd.exitChan)
	return nil
}
