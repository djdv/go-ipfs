package keyfs

import (
	"context"
	"errors"

	"github.com/ipfs/go-ipfs/mount/utils/transform"
	uio "github.com/ipfs/go-unixfs/io"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var (
	ErrKeyIsNotDir  = errors.New("operation on key that is not a directory")
	ErrKeyIsNotFile = errors.New("operation on key that is not a file")
	ErrDirNotEmpty  = errors.New("request to remove non-empty directory")
)

func Rmdir(ctx context.Context, core coreiface.CoreAPI, coreKey coreiface.Key) error {

	keyPath := coreKey.Path()

	iStat, _, err := transform.GetAttr(ctx, keyPath, core, transform.IPFSStatRequest{Type: true})
	if err != nil {
		return err
	}
	if iStat.FileType != coreiface.TDirectory {
		return ErrKeyIsNotDir
	}

	dirNode, err := core.ResolveNode(ctx, keyPath)
	if err != nil {
		return err
	}

	unixDir, err := uio.NewDirectoryFromNode(core.Dag(), dirNode)
	if err != nil {
		return err
	}

	dirCtx, cancel := context.WithCancel(ctx)

	_, ok := <-unixDir.EnumLinksAsync(dirCtx)
	if ok {
		cancel()
		return ErrDirNotEmpty
	}
	cancel()

	_, err = core.Key().Remove(ctx, coreKey.Name())
	return err
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
