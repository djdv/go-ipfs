// Package pinfs exposes an IPFS nodes pins as a 9P directory
package pinfs

import (
	"context"
	"runtime"

	"github.com/hugelgupf/p9/fsimpl/templatefs"
	"github.com/hugelgupf/p9/p9"
	common "github.com/ipfs/go-ipfs/mount/providers/9P/filesystems"
	"github.com/ipfs/go-ipfs/mount/providers/9P/filesystems/ipfs"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/pinfs"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var _ p9.File = (*File)(nil)
var _ common.WalkRef = (*File)(nil)

type File struct {
	templatefs.NoopFile
	p9.DefaultWalkGetAttr

	common.CoreBase
	common.OverlayBase

	dir           transform.Directory
	parent, proxy common.WalkRef
	open          bool // FIXME: this was added to the base class as an atomic value; we need to replace it here or there
}

func Attacher(ctx context.Context, core coreiface.CoreAPI, ops ...common.AttachOption) p9.Attacher {
	options := common.AttachOps(ops...)
	pd := &File{
		CoreBase: common.NewCoreBase("/pinfs", core, ops...),
		OverlayBase: common.OverlayBase{
			ParentCtx: ctx,
			Opened:    new(uintptr),
		},
		parent: options.Parent,
	}

	// set up our subsystem, used to relay walk names to IPFS
	subOpts := []common.AttachOption{
		common.Parent(pd),
		common.Logger(logging.Logger("IPFS")),
	}

	subsystem, err := ipfs.Attacher(ctx, core, subOpts...).Attach()
	if err != nil {
		panic(err)
	}

	pd.proxy = subsystem.(common.WalkRef)

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
		return p9.QID{}, 0, common.FileOpen
	}

	qid, err := pd.QID()
	if err != nil {
		return p9.QID{}, 0, err
	}

	pinDir, err := pinfs.OpenDir(pd.OperationsCtx, pd.Core)
	if err != nil {
		return p9.QID{}, 0, err
	}
	pd.dir = pinDir

	// atomic.StoreUintptr(pd.Opened, 1)
	pd.open = true

	return qid, common.UFS1BlockSize, nil
}

func (pd *File) Close() error {
	// NOTE: [refactoring] this value should be significant enough to tell if we're open or not
	// we still need close because of 9P semantics; (no re-open allowed)
	if pd.dir == nil {
		return common.FileNotOpen
	}

	// FIXME: not only is this check not atomic; it requires a critical section
	// we want a conditional CAS which is 1 step too many
	// if !open; return error; else swap value
	// we can't do a cas style upset since it will set an erroneous value
	if pd.open {
		//atomic.StoreUintptr(pd.Opened, 0)
	}

	pd.Closed = true
	pd.dir = nil

	if pd.FilesystemCancel != nil {
		pd.FilesystemCancel()
	}

	if pd.OperationsCancel != nil {
		pd.OperationsCancel()
	}

	return nil
}

func (pd *File) Readdir(offset uint64, count uint32) (p9.Dirents, error) {
	pd.Logger.Debugf("Readdir")

	if pd.dir == nil {
		return nil, common.FileNotOpen
	}

	//return common.FlatReaddir(pd.dir, offset, count)
	// TODO: review; quick hack to get building
	readCtx, cancel := context.WithCancel(pd.OperationsCtx)
	defer cancel()

	//return common.Readdir(readCtx, pd.Core, pd.CorePath(), pd.dir, offset)
	return common.Readdir(readCtx, pd.Core, common.RootPath("/ipfs"), pd.dir, offset)
}

/* WalkRef relevant */

func (pd *File) Fork() (common.WalkRef, error) {
	// make sure we were actually initialized
	if pd.FilesystemCtx == nil {
		return nil, common.FSCtxNotInitialized
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
func (pd *File) Step(name string) (common.WalkRef, error) {
	newFid, err := pd.proxy.Fork()
	if err != nil {
		return nil, err
	}
	return newFid.Step(name)
}

func (pd *File) CheckWalk() error {
	if pd.dir != nil {
		return common.FileOpen
	}
	return nil
}

func (pd *File) QID() (p9.QID, error) {
	return p9.QID{Type: p9.TypeDir,
		Path: common.CidToQIDPath(common.RootPath(pd.CoreNamespace).Cid()),
	}, nil
}

func (pd *File) Backtrack() (common.WalkRef, error) {
	if pd.parent != nil {
		return pd.parent, nil
	}
	return pd, nil
}

/* base class boilerplate */

func (pd *File) Walk(names []string) ([]p9.QID, p9.File, error) {
	pd.Logger.Debugf("Walk %q: %v", pd.String(), names)
	return common.Walker(pd, names)
}

func (pd *File) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	return p9.QID{
			Type: p9.TypeDir,
			Path: common.CidToQIDPath(common.RootPath(pd.CoreNamespace).Cid()),
		},
		p9.AttrMask{
			Mode: true,
		},
		p9.Attr{
			Mode: p9.ModeDirectory | common.IRXA,
		},
		nil
}

func (pd *File) clone() (*File, error) {
	// make sure we were actually initialized
	if pd.ParentCtx == nil {
		return nil, common.FSCtxNotInitialized
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
