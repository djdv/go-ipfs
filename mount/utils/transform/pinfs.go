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
	core               coreiface.CoreAPI
	ctx                context.Context
	err                error
	snapshot           []coreiface.Pin
	currentPos, endPos uint64
}

func (pd *pinDir) To9P() (p9.Dirents, error) {
	if pd.err != nil {
		return nil, pd.err
	}

	subSlice := pd.snapshot[pd.currentPos:pd.endPos]
	nineEnts := make(p9.Dirents, 0, len(subSlice))

	for _, pin := range subSlice {
		callCtx, cancel := context.WithTimeout(pd.ctx, 10*time.Second)
		subQid, err := coreToQID(callCtx, pin.Path(), pd.core)
		if err != nil {
			cancel()
			pd.err = err
			return nil, err
		}

		pd.currentPos++

		nineEnts = append(nineEnts, p9.Dirent{
			Name:   gopath.Base(pin.Path().String()),
			Offset: pd.currentPos,
			QID:    subQid,
		})
		cancel()
	}

	return nineEnts, nil
}

func (pd *pinDir) ToFuse() (<-chan FuseStatGroup, error) {
	if pd.err != nil {
		return nil, pd.err
	}

	subSlice := pd.snapshot[pd.currentPos:pd.endPos]
	dirChan := make(chan FuseStatGroup)

	go func() {
		defer close(dirChan)
		for _, pin := range subSlice {
			select {
			case <-pd.ctx.Done():
				pd.err = pd.ctx.Err()
				break
			default:
			}

			pd.currentPos++

			var fStat *fuselib.Stat_t
			if CanReaddirPlus {
				callCtx, cancel := context.WithTimeout(pd.ctx, 10*time.Second)
				iStat, _, err := GetAttrCore(callCtx, pin.Path(), pd.core, statRequestAll)
				cancel()

				// stat errors are not fatal; it's okay to return nil to fill
				// it just means the OS will call getattr on this entry later
				if err == nil {
					fStat = iStat.ToFuse()
				}
			}

			dirChan <- FuseStatGroup{
				gopath.Base(pin.Path().String()),
				int64(pd.currentPos),
				fStat,
			}
		}
	}()
	return dirChan, nil
}

func (pd *pinDir) Read(offset, count uint64) directoryState {
	if pd.err != nil { // refuse to operate
		return pd
	}

	if pd.snapshot == nil {
		pd.err = errors.New("not opened") // TODO: replace err
		return pd
	}

	sLen := uint64(len(pd.snapshot))

	if offset > sLen {
		pd.err = fmt.Errorf(errSeekFmt, offset, sLen)
		return pd
	}

	var end uint64
	if count == 0 { // special case, returns all
		end = sLen
	} else {
		end = uint64(offset + count)
		if end > sLen { // rebound
			end = sLen
		}
	}

	pd.currentPos = uint64(offset)
	pd.endPos = end
	return pd
}

func (pd *pinDir) Seek(offset uint64) error {
	if pd.err != nil { // refuse to operate
		return pd.err
	}

	if sLen := uint64(len(pd.snapshot)); offset > sLen {
		return fmt.Errorf(errSeekFmt, offset, sLen)
	}

	pd.currentPos = offset
	return nil
}

func OpenDirPinfs(ctx context.Context, core coreiface.CoreAPI) (Directory, error) {
	pins, err := core.Pin().Ls(ctx, coreoptions.Pin.Type.Recursive())
	if err != nil {
		return nil, err
	}

	return &pinDir{
		core:     core,
		ctx:      ctx,
		snapshot: pins,
	}, nil
}

func (pd *pinDir) Close() error {
	pd.snapshot = nil
	return nil
}
