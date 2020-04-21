package transform

import (
	"context"
	"errors"
	"fmt"
	gopath "path"
	"time"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/hugelgupf/p9/p9"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	coreoptions "github.com/ipfs/interface-go-ipfs-core/options"
)

var (
	_ Directory      = (*pinDir)(nil)
	_ directoryState = (*pinDir)(nil)
)

// TODO: [async safe]
// ToFuse should lock until it's done with its goroutine
type pinDir struct {
	core coreiface.CoreAPI
	ctx  context.Context
	err  error

	pins, pinSlice []coreiface.Pin
	cursor         uint64
}

func (pd *pinDir) To9P() (p9.Dirents, error) {
	if pd.err != nil {
		return nil, pd.err
	}

	nineEnts := make(p9.Dirents, 0, len(pd.pinSlice))
	for _, pin := range pd.pinSlice {
		callCtx, cancel := context.WithTimeout(pd.ctx, 10*time.Second)
		subQid, err := coreToQID(callCtx, pin.Path(), pd.core)
		if err != nil {
			cancel()
			pd.err = err
			return nil, err
		}

		nineEnts = append(nineEnts, p9.Dirent{
			Name:   gopath.Base(pin.Path().String()),
			Offset: pd.cursor,
			QID:    subQid,
		})

		pd.cursor++
		cancel()
	}

	return nineEnts, pd.err
}

func (pd *pinDir) ToFuse() (<-chan FuseStatGroup, error) {
	if pd.err != nil {
		return nil, pd.err
	}

	dirChan := make(chan FuseStatGroup)
	go func() {
		defer close(dirChan)
		for _, pin := range pd.pinSlice {
			select {
			case <-pd.ctx.Done():
				pd.err = pd.ctx.Err()
				break
			default:
			}

			var fStat *fuselib.Stat_t
			if CanReaddirPlus {
				callCtx, cancel := context.WithTimeout(pd.ctx, 10*time.Second)
				iStat, _, err := GetAttrCore(callCtx, pin.Path(), pd.core, IPFSStatRequestAll)
				cancel()

				// stat errors are not fatal; it's okay to return nil to fill
				// it just means the OS will call getattr on this entry later
				if err == nil {
					fStat = iStat.ToFuse()
				}
			}

			dirChan <- FuseStatGroup{
				gopath.Base(pin.Path().String()),
				int64(pd.cursor),
				fStat,
			}

			pd.cursor++
		}
	}()
	return dirChan, pd.err
}

func (pd *pinDir) Read(offset, count uint64) directoryState {
	if pd.err != nil { // refuse to operate
		return pd
	}

	if offset == 0 { // (re)init
		pins, err := pd.core.Pin().Ls(pd.ctx, coreoptions.Pin.Type.Recursive())
		if err != nil {
			pd.err = err
			return pd
		}
		pd.pins = pins
		pd.cursor = 1
	}

	if pd.pins == nil {
		pd.err = errors.New("not opened") // TODO: replace err
		return pd
	}

	if offset != pd.cursor-1 { // offset provided to us, was previously provided by us; or has since been invalidated
		pd.err = fmt.Errorf("read offset %d is not valid", offset)
		return pd
	}

	var (
		sLen = uint64(len(pd.pins))
		end  uint64
	)
	if count == 0 { // special case, returns all
		end = sLen
	} else {
		end = offset + count
		if end > sLen { // rebound
			end = sLen
		}
	}

	pd.pinSlice = pd.pins[offset:end]
	return pd
}

func OpenDirPinfs(ctx context.Context, core coreiface.CoreAPI) Directory {
	return &pinDir{
		core: core,
		ctx:  ctx,
	}
}

func (pd *pinDir) Close() error {
	pd.pins = nil
	return nil
}
