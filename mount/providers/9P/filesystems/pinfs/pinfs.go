// Package pinfs exposes an IPFS nodes pins as a 9P directory
package pinfs

import (
	"context"
	gopath "path"
	"runtime"
	"sync/atomic"

	"github.com/hugelgupf/p9/p9"
	"github.com/hugelgupf/p9/fsimpl/templatefs"
	fserrors "github.com/ipfs/go-ipfs/mount/providers/9P/errors"
	"github.com/ipfs/go-ipfs/mount/providers/9P/filesystems/ipfs"
	"github.com/ipfs/go-ipfs/mount/providers/9P/meta"
	fsutils "github.com/ipfs/go-ipfs/mount/providers/9P/utils"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	coreoptions "github.com/ipfs/interface-go-ipfs-core/options"
)

var _ p9.File = (*File)(nil)
var _ meta.WalkRef = (*File)(nil)

type File struct {
	templatefs.NoopFile
	p9.DefaultWalkGetAttr

	meta.CoreBase
	meta.OverlayBase

	ents          p9.Dirents
	parent, proxy meta.WalkRef
	open          bool
}

func Attacher(ctx context.Context, core coreiface.CoreAPI, ops ...meta.AttachOption) p9.Attacher {
	options := meta.AttachOps(ops...)
	pd := &File{
		CoreBase: meta.NewCoreBase("/pinfs", core, ops...),
		OverlayBase: meta.OverlayBase{
			ParentCtx: ctx,
			Opened:    new(uintptr),
		},
		parent: options.Parent,
	}

	// set up our subsystem, used to relay walk names to IPFS
	subOpts := []meta.AttachOption{
		meta.Parent(pd),
		meta.Logger(logging.Logger("IPFS")),
	}

	subsystem, err := ipfs.Attacher(ctx, core, subOpts...).Attach()
	if err != nil {
		panic(err)
	}

	pd.proxy = subsystem.(meta.WalkRef)

	// detach from our proxied system when we fall out of memory
	runtime.SetFinalizer(pd, func(pinRoot *File) {
		pinRoot.proxy.Close()
	})

	return pd
}

func (pd *File) Attach() (p9.File, error) {
	pd.Logger.Debugf("Attach")

	newFid, err := pd.clone()
	if err != nil {
		return nil, err
	}

	newFid.FilesystemCtx, newFid.FilesystemCancel = context.WithCancel(newFid.ParentCtx)
	return newFid, nil
}

func (pd *File) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	pd.Logger.Debugf("Open: %s", pd.String())

	if pd.IsOpen() {
		return p9.QID{}, 0, fserrors.FileOpen
	}

	qid, err := pd.QID()
	if err != nil {
		return p9.QID{}, 0, err
	}

	// IPFS core representation
	pins, err := pd.Core.Pin().Ls(pd.OperationsCtx, coreoptions.Pin.Type.Recursive())
	if err != nil {
		return qid, 0, err
	}

	// 9P representation
	pd.ents = make(p9.Dirents, 0, len(pins))

	// actual conversion
	for i, pin := range pins {
		callCtx, cancel := pd.CallCtx()
		subQid, err := meta.CoreToQID(callCtx, pin.Path(), pd.Core)
		if err != nil {
			cancel()
			return p9.QID{}, 0, err
		}

		pd.ents = append(pd.ents, p9.Dirent{
			Name:   gopath.Base(pin.Path().String()),
			Offset: uint64(i + 1),
			QID:    subQid,
		})
		cancel()
	}

	atomic.StoreUintptr(pd.Opened, 1)
	pd.open = true

	return qid, meta.UFS1BlockSize, nil
}

func (pd *File) Close() error {
	pd.Closed = true
	pd.ents = nil

	if pd.FilesystemCancel != nil {
		pd.FilesystemCancel()
	}

	if pd.OperationsCancel != nil {
		pd.OperationsCancel()
	}

	if pd.open {
		atomic.StoreUintptr(pd.Opened, 0)
	}

	return nil
}

func (pd *File) Readdir(offset uint64, count uint32) (p9.Dirents, error) {
	pd.Logger.Debugf("Readdir")

	if pd.ents == nil {
		return nil, fserrors.FileNotOpen
	}

	return meta.FlatReaddir(pd.ents, offset, count)
}

/* WalkRef relevant */

func (pd *File) Fork() (meta.WalkRef, error) {
	// make sure we were actually initalized
	if pd.FilesystemCtx == nil {
		return nil, fserrors.FSCtxNotInitalized
	}

	// and also not canceled / still valid
	if err := pd.FilesystemCtx.Err(); err != nil {
		return nil, err
	}

	newFid, err := pd.clone()
	if err != nil {
		return nil, err
	}

	newFid.OperationsCtx, newFid.OperationsCancel = context.WithCancel(newFid.FilesystemCtx)

	return newFid, nil
}

// PinFS forks the IPFS root that was set during construction
// and calls step on it rather than itself
func (pd *File) Step(name string) (meta.WalkRef, error) {
	newFid, err := pd.proxy.Fork()
	if err != nil {
		return nil, err
	}
	return newFid.Step(name)
}

func (pd *File) CheckWalk() error {
	if pd.ents != nil {
		return fserrors.FileOpen
	}
	return nil
}

func (pd *File) QID() (p9.QID, error) {
	return p9.QID{Type: p9.TypeDir,
		Path: meta.CidToQIDPath(meta.RootPath(pd.CoreNamespace).Cid()),
	}, nil
}

func (pd *File) Backtrack() (meta.WalkRef, error) {
	if pd.parent != nil {
		return pd.parent, nil
	}
	return pd, nil
}

/* base class boilerplate */

func (pd *File) Walk(names []string) ([]p9.QID, p9.File, error) {
	pd.Logger.Debugf("Walk %q: %v", pd.String(), names)
	return fsutils.Walker(pd, names)
}

func (pd *File) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	return p9.QID{
			Type: p9.TypeDir,
			Path: meta.CidToQIDPath(meta.RootPath(pd.CoreNamespace).Cid()),
		},
		p9.AttrMask{
			Mode: true,
		},
		p9.Attr{
			Mode: p9.ModeDirectory | meta.IRXA,
		},
		nil
}

func (pd *File) clone() (*File, error) {
	// make sure we were actually initalized
	if pd.ParentCtx == nil {
		return nil, fserrors.FSCtxNotInitalized
	}

	// and also not canceled / still valid
	if err := pd.ParentCtx.Err(); err != nil {
		return nil, err
	}

	// all good; derive a new reference from this instance and return it
	return &File{
		CoreBase:    pd.CoreBase,
		OverlayBase: pd.OverlayBase.Clone(),
		parent:      pd.parent,
		proxy:       pd.proxy,
	}, nil
}
