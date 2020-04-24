package pinfs

import (
	"context"
	gopath "path"
	"time"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/hugelgupf/p9/p9"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/ipfscore"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/ipld"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	coreoptions "github.com/ipfs/interface-go-ipfs-core/options"
)

var (
	_ transform.Directory      = (*pinDir)(nil)
	_ transform.DirectoryState = (*pinDir)(nil)
)

// TODO: [async safe]
// ToFuse should lock until it's done with its goroutine
type pinDir struct {
	core coreiface.CoreAPI
	ctx  context.Context
	err  error

	pins, pinSlice           []coreiface.Pin
	cursor, validOffsetBound uint64 // See Filldir remark [53efa63b-7d75-4a5c-96c9-47e2dc7c6e6b] for directory bound info
}

func (pd *pinDir) To9P() (p9.Dirents, error) {
	if pd.err != nil {
		return nil, pd.err
	}

	nineEnts := make(p9.Dirents, 0, len(pd.pinSlice))
	for _, pin := range pd.pinSlice {
		callCtx, cancel := context.WithTimeout(pd.ctx, 10*time.Second)

		pinPath := pin.Path()
		node, err := pd.core.Dag().Get(callCtx, pinPath.Cid())
		if err != nil {
			pd.err = err
			cancel()
			return nil, err
		}

		stat, _, err := ipld.GetAttr(callCtx, node, transform.IPFSStatRequest{Type: true})
		if err != nil {
			pd.err = err
			cancel()
			return nil, err
		}

		nineEnts = append(nineEnts, p9.Dirent{
			Name:   gopath.Base(pin.Path().String()),
			Offset: pd.cursor,
			QID:    transform.CidToQID(pinPath.Cid(), stat.FileType),
		})

		pd.cursor++
		pd.validOffsetBound++
		cancel()
	}

	return nineEnts, pd.err
}

func (pd *pinDir) ToFuse() (<-chan transform.FuseStatGroup, error) {
	if pd.err != nil {
		return nil, pd.err
	}

	dirChan := make(chan transform.FuseStatGroup)
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
			if provcom.CanReaddirPlus {
				callCtx, cancel := context.WithTimeout(pd.ctx, 10*time.Second)
				iStat, _, err := ipfscore.GetAttr(callCtx, pin.Path(), pd.core, transform.IPFSStatRequestAll)
				cancel()

				// stat errors are not fatal; it's okay to return nil to fill
				// it just means the OS will call getattr on this entry later
				if err == nil {
					fStat = iStat.ToFuse()
				}
			}

			dirChan <- transform.FuseStatGroup{
				Name:   gopath.Base(pin.Path().String()),
				Offset: int64(pd.cursor),
				Stat:   fStat,
			}

			pd.cursor++
			pd.validOffsetBound++
		}
	}()
	return dirChan, pd.err
}

func (pd *pinDir) Readdir(offset, count uint64) transform.DirectoryState {
	if pd.err != nil { // refuse to operate
		return pd
	}

	// reinit // rewinddir
	if offset == 0 && pd.cursor != 1 { // only reset if we've actually moved
		pins, err := pd.core.Pin().Ls(pd.ctx, coreoptions.Pin.Type.Recursive())
		if err != nil {
			pd.err = err
			return pd
		}

		pd.pins = pins
		pd.cursor = 1
	}

	if offset < pd.validOffsetBound || offset > pd.cursor {
		// return NULL dirent to reader
		pd.pinSlice = make([]coreiface.Pin, 0)
		return pd
	}

	if offset > 0 { // convert the telldir token within our valid range, back to a real offset
		offset %= pd.validOffsetBound
		pd.cursor = offset + 1
	}

	var (
		sLen = uint64(len(pd.pins))
		end  uint64
	)

	if count == 0 { // special case, returns all
		end = sLen
	} else if end = offset + count; end > sLen { // cap to <= len
		end = sLen
	}

	pd.pinSlice = pd.pins[offset:end]
	return pd
}

func OpenDir(ctx context.Context, core coreiface.CoreAPI) (*pinDir, error) {
	pins, err := core.Pin().Ls(ctx, coreoptions.Pin.Type.Recursive()) // direct pins should take the least effort
	if err != nil {
		return nil, err
	}

	return &pinDir{
		core:   core,
		ctx:    ctx,
		cursor: 1,
		pins:   pins,
	}, nil
}

func (pd *pinDir) Close() error {
	pd.pins = nil
	return nil
}
