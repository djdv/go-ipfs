package keyfs

import (
	"context"
	"errors"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	uio "github.com/ipfs/go-unixfs/io"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var (
	ErrKeyIsNotDir  = errors.New("operation on key that is not a directory")
	ErrKeyIsNotFile = errors.New("operation on key that is not a file")
)

//TODO: all the 9P errors
func Rmdir(ctx context.Context, core coreiface.CoreAPI, coreKey coreiface.Key) transform.Error {

	keyPath := coreKey.Path()

	iStat, _, err := transform.GetAttr(ctx, keyPath, core, transform.IPFSStatRequest{Type: true})
	if err != nil {
		return &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.ENOENT,
			P9pErr: err,
		}
	}
	if iStat.FileType != coreiface.TDirectory {
		return &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.ENOTDIR,
			P9pErr: err,
		}
	}

	dirNode, err := core.ResolveNode(ctx, keyPath)
	if err != nil {
		return &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.ENOENT,
			P9pErr: err,
		}
	}

	unixDir, err := uio.NewDirectoryFromNode(core.Dag(), dirNode)
	if err != nil {
		return &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.ENOENT,
			P9pErr: err,
		}
	}

	dirCtx, cancel := context.WithCancel(ctx)

	_, ok := <-unixDir.EnumLinksAsync(dirCtx)
	if ok {
		cancel()
		return &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.ENOTEMPTY,
			P9pErr: err,
		}
	}
	cancel()

	if _, err = core.Key().Remove(ctx, coreKey.Name()); err != nil {
		return &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.EIO,
			P9pErr: err,
		}
	}
	return nil
}

func Unlink(ctx context.Context, core coreiface.CoreAPI, coreKey coreiface.Key) error {
	keyPath := coreKey.Path()

	iStat, _, err := transform.GetAttr(ctx, keyPath, core, transform.IPFSStatRequest{Type: true})
	if err != nil {
		return err
	}
	if iStat.FileType != coreiface.TFile {
		return ErrKeyIsNotFile
	}

	_, err = core.Key().Remove(ctx, coreKey.Name())
	return err
}
