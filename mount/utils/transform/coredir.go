package transform

import (
	"context"
	"errors"
	"fmt"
	"time"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/hugelgupf/p9/p9"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

var (
	_ Directory      = (*coreDir)(nil)
	_ directoryState = (*coreDir)(nil)
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

	path      corepath.Path
	entryChan entryChan
	exitChan  exitChan
	cursor    uint64
}

func (cd *coreDir) To9P() (p9.Dirents, error) {
	if cd.err != nil {
		return nil, cd.err
	}

	nineEnts := make(p9.Dirents, 0)
	for coreEntry := range cd.exitChan {
		// convert from core wrapper -> 9P
		nineEnt := coreDirEntryTo9Dirent(coreEntry.DirEntry)
		nineEnt.Offset = coreEntry.offset

		nineEnts = append(nineEnts, nineEnt)
	}

	return nineEnts, cd.err
}

func (cd *coreDir) ToFuse() (<-chan FuseStatGroup, error) {
	if cd.err != nil {
		return nil, cd.err
	}

	dirChan := make(chan FuseStatGroup)

	go func() {
		defer close(dirChan)
		for coreEntry := range cd.exitChan {

			var fStat *fuselib.Stat_t
			if CanReaddirPlus {
				callCtx, cancel := context.WithTimeout(cd.ctx, 10*time.Second)

				subPath := corepath.Join(cd.path, coreEntry.DirEntry.Name)
				iStat, _, err := GetAttrCore(callCtx, subPath, cd.core, IPFSStatRequestAll)
				cancel()

				// stat errors are not fatal; it's okay to return nil to fill
				// it just means the OS will call getattr on this entry later
				if err == nil {
					fStat = iStat.ToFuse()
				}
			}

			dirChan <- FuseStatGroup{
				coreEntry.Name,
				int64(coreEntry.offset),
				fStat,
			}
		}
	}()
	return dirChan, cd.err
}

func (cd *coreDir) Readdir(offset, count uint64) directoryState {
	if cd.err != nil { // refuse to operate
		return cd
	}

	if offset == 0 { // initialize
		if cd.cancel != nil { // close previous request (if any)
			cd.cancel()
		}
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

	if cd.entryChan == nil {
		cd.err = errors.New("not opened") // TODO: replace err
		return cd
	}

	if offset != cd.cursor-1 { // offset provided to us, was previously provided by us; or has since been invalidated
		cd.err = fmt.Errorf("read offset %d is not valid", offset)
		return cd
	}

	untilEndOfStream := count == 0

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

			case <-cd.ctx.Done():
				cd.err = cd.ctx.Err()
				return
			}
		}
	}()
	return cd
}

func CoreOpenDir(ctx context.Context, path corepath.Path, core coreiface.CoreAPI) (Directory, error) {
	// do type checking of path
	iStat, _, err := GetAttrCore(ctx, path, core, IPFSStatRequest{Type: true})
	if err != nil {
		return nil, err
	}

	if iStat.FileType != coreiface.TDirectory {
		// TODO: [ad4c44e0-a93f-4333-92d2-7a2aeccce3ef] typedef errors
		return nil, fmt.Errorf("%q (type: %s) is not a diretory", path.String(), iStat.FileType.String())
	}

	return &coreDir{
		core: core,
		ctx:  ctx,
		path: path,
	}, nil
}

func (cd *coreDir) Close() error {
	cd.exitChan = nil
	return nil
}
