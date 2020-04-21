// Package ipfs exposes the Inter-Planetary File System API as a 9P compatible resource server
package ipfs

import (
	"context"
	"fmt"
	"io"

	"github.com/hugelgupf/p9/fsimpl/templatefs"
	"github.com/hugelgupf/p9/p9"
	common "github.com/ipfs/go-ipfs/mount/providers/9P/filesystems"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var _ p9.File = (*File)(nil)
var _ common.WalkRef = (*File)(nil)

// The IPFS File exposes the IPFS API over a p9.File interface
// Walk does not expect a namespace, only path arguments
// e.g. `ipfs.Walk([]string("Qm...", "subdir")` not `ipfs.Walk([]string("ipfs", "Qm...", "subdir")`
type File struct {
	templatefs.NoopFile
	p9.DefaultWalkGetAttr

	common.CoreBase
	common.OverlayBase

	// operation handle storage
	file      transform.File
	directory transform.Directory

	// optional WalkRef extension node
	// (".." on root gets sent here if set)
	Parent common.WalkRef
}

func Attacher(ctx context.Context, core coreiface.CoreAPI, ops ...common.AttachOption) p9.Attacher {
	options := common.AttachOps(ops...)
	return &File{
		CoreBase:    common.NewCoreBase("/ipfs", core, ops...),
		OverlayBase: common.OverlayBase{ParentCtx: ctx},
		Parent:      options.Parent,
	}
}

func (id *File) Attach() (p9.File, error) {
	id.Logger.Debugf("Attach")

	newFid, err := id.clone()
	if err != nil {
		return nil, err
	}

	newFid.FilesystemCtx, newFid.FilesystemCancel = context.WithCancel(newFid.ParentCtx)
	return newFid, nil
}

func (id *File) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	id.Logger.Debugf("GetAttr")
	if len(id.Trail) == 0 { // root entry
		return p9.QID{
				Type: p9.TypeDir,
				Path: common.CidToQIDPath(common.RootPath(id.CoreNamespace).Cid()),
			},
			p9.AttrMask{
				Mode: true,
			},
			p9.Attr{
				Mode: p9.ModeDirectory | common.IRXA,
			},
			nil
	}

	// all other entries are looked up
	callCtx, cancel := id.CallCtx()
	defer cancel()

	qid, err := common.CoreToQID(callCtx, id.CorePath(), id.Core)
	if err != nil {
		return p9.QID{}, p9.AttrMask{}, p9.Attr{}, err
	}

	var attr p9.Attr
	filled, err := common.CoreToAttr(callCtx, &attr, id.CorePath(), id.Core, req)
	if err != nil {
		id.Logger.Error(err)
		return qid, filled, attr, err
	}

	// add metadata not contained in IPFS-UFS v1 objects
	if req.RDev { // device
		attr.RDev, filled.RDev = common.DevIPFS, true
	}

	if req.Mode { // UFS provides type bits, we provide permission bits
		attr.Mode |= common.IRXA
	}

	return qid, filled, attr, err
}

func (id *File) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	id.Logger.Debugf("Open: %s", id.String())

	qid, err := id.QID()
	if err != nil {
		return p9.QID{}, 0, err
	}

	if qid.Type == p9.TypeDir { // handle directories
		if len(id.Trail) == 0 { // handle the root itself (empty)
			*id.Opened = 1 // FIXME: [69419872-3e02-4b04-9d23-dce6318b7fb2]
			return qid, 0, nil
		}

		// everything else
		dir, err := transform.CoreOpenDir(id.OperationsCtx, id.CorePath(), id.Core)
		if err != nil {
			return qid, 0, err
		}

		id.directory = dir
		*id.Opened = 1 // FIXME: [69419872-3e02-4b04-9d23-dce6318b7fb2]
		return qid, 0, nil
	}

	callCtx, cancel := id.CallCtx()
	defer cancel()

	file, err := transform.CoreOpenFile(callCtx, id.CorePath(), id.Core, transform.IOFlagsFrom9P(mode))
	if err != nil {
		return qid, 0, err
	}
	id.file = file

	return qid, common.UFS1BlockSize, nil
}

// Close invalidates all references of the node
func (id *File) Close() error {
	id.Closed = true

	var err error // log all errors, return the last one

	if id.file != nil {
		//TODO: timeout and cancel the context if Close takes too long
		if err = id.file.Close(); err != nil {
			id.Logger.Error(err)
		}
		id.file = nil
	}
	id.directory = nil

	if id.FilesystemCancel != nil {
		id.FilesystemCancel()
	}

	if id.OperationsCancel != nil {
		id.OperationsCancel()
	}

	// [69419872-3e02-4b04-9d23-dce6318b7fb2] open/close async refactor
	id.Closed = true
	*id.Opened = 0

	return err
}

func (id *File) Readdir(offset uint64, count uint32) (p9.Dirents, error) {
	id.Logger.Debugf("Readdir %q %d %d", id.String(), offset, count)

	if *id.Opened == 0 { // FIXME: [69419872-3e02-4b04-9d23-dce6318b7fb2]
		return nil, fmt.Errorf("directory %q is not open for reading", id.String())
	}

	// special case for root
	if len(id.Trail) == 0 {
		return nil, nil
	}

	if id.directory == nil {
		return nil, common.FileNotOpen
	}

	//return common.FlatReaddir(pd.dir, offset, count)
	return id.directory.Read(offset, uint64(count)).To9P()
}

func (id *File) ReadAt(p []byte, offset int64) (int, error) {
	const readAtFmtErr = "ReadAt {%d}%q: %s"

	if id.file == nil {
		id.Logger.Errorf(readAtFmtErr, offset, id.String(), common.FileNotOpen)
		return 0, common.FileNotOpen
	}

	size, err := id.file.Size()
	if err != nil {
		id.Logger.Errorf(readAtFmtErr, offset, id.String(), err)
		return 0, err
	}

	// FIXME: unsafe compare; maxsize
	if offset >= size {
		//NOTE [styx]: If the offset field is greater than or equal to the number of bytes in the file, a count of zero will be returned.
		return 0, io.EOF
	}

	if _, err := id.file.Seek(int64(offset), io.SeekStart); err != nil {
		//id.OperationsCancel()
		id.Logger.Errorf(readAtFmtErr, offset, id.String(), err)
		return 0, err
	}

	readBytes, err := id.file.Read(p)
	if err != nil && err != io.EOF {
		id.Logger.Errorf(readAtFmtErr, offset, id.String(), err)
	}

	return readBytes, err
}

func (id *File) Walk(names []string) ([]p9.QID, p9.File, error) {
	id.Logger.Debugf("Walk %q: %v", id.String(), names)
	return common.Walker(id, names)
}

/* WalkRef relevant */

func (id *File) CheckWalk() error {
	if id.file != nil || id.directory != nil {
		return common.FileOpen
	}

	return nil
}

func (id *File) Fork() (common.WalkRef, error) {
	// make sure we were actually initalized
	if id.FilesystemCtx == nil {
		return nil, common.FSCtxNotInitalized
	}

	// and also not canceled / still valid
	if err := id.FilesystemCtx.Err(); err != nil {
		return nil, err
	}

	newFid, err := id.clone()
	if err != nil {
		return nil, err
	}

	newFid.OperationsCtx, newFid.OperationsCancel = context.WithCancel(newFid.FilesystemCtx)

	return newFid, nil
}

// Step appends "name" to the File's current path, and returns itself
func (id *File) Step(name string) (common.WalkRef, error) {
	qid, err := id.QID()
	if err != nil {
		return nil, err
	}

	if qid.Type != p9.TypeDir {
		return nil, common.ENOTDIR
	}

	tLen := len(id.Trail)
	id.Trail = append(id.Trail[:tLen:tLen], name)
	return id, nil
}

func (id *File) QID() (p9.QID, error) {
	if len(id.Trail) == 0 {
		return p9.QID{
			Type: p9.TypeDir,
			Path: common.CidToQIDPath(common.RootPath(id.CoreNamespace).Cid()),
		}, nil
	}

	callCtx, cancel := id.CallCtx()
	defer cancel()

	return common.CoreToQID(callCtx, id.CorePath(), id.Core)
}

func (id *File) Backtrack() (common.WalkRef, error) {
	// if we're a root return our parent, or ourselves if we don't have one
	if len(id.Trail) == 0 {
		if id.Parent != nil {
			return id.Parent, nil
		}
		return id, nil
	}

	// otherwise step back
	id.Trail = id.Trail[:len(id.Trail)-1]
	return id, nil
}

func (id *File) clone() (*File, error) {
	// make sure we were actually initalized
	if id.ParentCtx == nil {
		return nil, common.FSCtxNotInitalized
	}

	// and also not canceled / still valid
	if err := id.ParentCtx.Err(); err != nil {
		return nil, err
	}

	// all good; derive a new reference from this instance and return it
	return &File{
		CoreBase: id.CoreBase,
		OverlayBase: common.OverlayBase{
			ParentCtx:     id.ParentCtx,
			FilesystemCtx: id.FilesystemCtx,
		},
		Parent: id.Parent,
	}, nil
}
