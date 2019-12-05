package ipfs

import (
	"context"
	"fmt"
	"io"

	"github.com/hugelgupf/p9/p9"
	"github.com/hugelgupf/p9/unimplfs"
	files "github.com/ipfs/go-ipfs-files"
	fserrors "github.com/ipfs/go-ipfs/plugin/plugins/filesystem/errors"
	"github.com/ipfs/go-ipfs/plugin/plugins/filesystem/meta"
	fsutils "github.com/ipfs/go-ipfs/plugin/plugins/filesystem/utils"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var _ p9.File = (*File)(nil)
var _ meta.WalkRef = (*File)(nil)

// The IPFS File exposes the IPFS API over a p9.File interface (as a directory)
// Walk does not expect a namespace, only path arguments
// e.g. `ipfs.Walk([]string("Qm...", "subdir")` not `ipfs.Walk([]string("ipfs", "Qm...", "subdir")`
type File struct {
	unimplfs.NoopFile
	p9.DefaultWalkGetAttr

	meta.CoreBase
	meta.OverlayBase

	// operation handle storage
	file      files.File
	directory *directoryStream

	// optional WalkRef extension node
	// (".." on root gets sent here if set)
	Parent meta.WalkRef
}

type directoryStream struct {
	entryChan <-chan coreiface.DirEntry
	cursor    uint64
	err       error
}

func Attacher(ctx context.Context, core coreiface.CoreAPI, ops ...meta.AttachOption) p9.Attacher {
	options := meta.AttachOps(ops...)
	return &File{
		CoreBase:    meta.NewCoreBase("/ipfs", core, ops...),
		OverlayBase: meta.OverlayBase{ParentCtx: ctx},
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
				Path: meta.CidToQIDPath(meta.RootPath(id.CoreNamespace).Cid()),
			},
			p9.AttrMask{
				Mode: true,
			},
			p9.Attr{
				Mode: p9.ModeDirectory | meta.IRXA,
			},
			nil
	}

	// all other entries are looked up
	callCtx, cancel := id.CallCtx()
	defer cancel()

	qid, err := meta.CoreToQID(callCtx, id.CorePath(), id.Core)
	if err != nil {
		return p9.QID{}, p9.AttrMask{}, p9.Attr{}, err
	}

	var attr p9.Attr
	filled, err := meta.CoreToAttr(callCtx, &attr, id.CorePath(), id.Core, req)
	if err != nil {
		id.Logger.Error(err)
		return qid, filled, attr, err
	}

	// add metadata not contained in IPFS-UFS v1 objects
	if req.RDev { // device
		attr.RDev, filled.RDev = meta.DevIPFS, true
	}

	if req.Mode { // UFS provides type bits, we provide permission bits
		attr.Mode |= meta.IRXA
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
			id.directory = &directoryStream{}
			return qid, 0, nil
		}

		// everything else
		c, err := id.Core.Unixfs().Ls(id.OperationsCtx, id.CorePath())
		if err != nil {
			return qid, 0, err
		}

		id.directory = &directoryStream{entryChan: c}
		return qid, 0, nil
	}

	// handle files
	apiNode, err := id.Core.Unixfs().Get(id.OperationsCtx, id.CorePath())
	if err != nil {
		return qid, 0, err
	}

	fileNode, ok := apiNode.(files.File)
	if !ok {
		return qid, 0, fmt.Errorf("%q does not appear to be a file: %T", id.String(), apiNode)
	}
	id.file = fileNode

	return qid, meta.UFS1BlockSize, nil
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

	return err
}

func (id *File) Readdir(offset uint64, count uint32) ([]p9.Dirent, error) {
	id.Logger.Debugf("Readdir %q %d %d", id.String(), offset, count)

	if id.directory == nil {
		return nil, fmt.Errorf("directory %q is not open for reading", id.String())
	}

	if id.directory.err != nil { // previous request must have failed
		return nil, id.directory.err
	}

	// special case for root
	if len(id.Trail) == 0 {
		return nil, nil
	}

	if offset < id.directory.cursor {
		return nil, fmt.Errorf("read offset %d is behind current entry %d, seeking backwards in directory streams is not supported", offset, id.directory.cursor)
	}

	ents := make([]p9.Dirent, 0)

	for len(ents) < int(count) {
		select {
		case entry, open := <-id.directory.entryChan:
			if !open {
				//id.OperationsCancel()
				return ents, nil
			}
			if entry.Err != nil {
				id.directory.err = entry.Err
				return nil, entry.Err
			}

			// we consumed an entry
			id.directory.cursor++

			// skip processing the entry if its below the request offset
			if offset > id.directory.cursor {
				continue
			}
			nineEnt, err := meta.CoreEntTo9Ent(entry)
			if err != nil {
				id.directory.err = err
				return nil, err
			}
			nineEnt.Offset = id.directory.cursor
			ents = append(ents, nineEnt)

		case <-id.FilesystemCtx.Done():
			id.directory.err = id.FilesystemCtx.Err()
			id.Logger.Error(id.directory.err)
			return ents, id.directory.err
		}
	}

	id.Logger.Debugf("Readdir returning [%d]%v\n", len(ents), ents)
	return ents, nil
}

func (id *File) ReadAt(p []byte, offset uint64) (int, error) {
	const readAtFmtErr = "ReadAt {%d}%q: %s"

	if id.file == nil {
		id.Logger.Errorf(readAtFmtErr, offset, id.String(), fserrors.FileNotOpen)
		return 0, fserrors.FileNotOpen
	}

	size, err := id.file.Size()
	if err != nil {
		id.Logger.Errorf(readAtFmtErr, offset, id.String(), err)
		return 0, err
	}

	if offset >= uint64(size) {
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
	return fsutils.Walker(id, names)
}

/* WalkRef relevant */

func (id *File) CheckWalk() error {
	if id.file != nil || id.directory != nil {
		return fserrors.FileOpen
	}

	return nil
}

func (id *File) Fork() (meta.WalkRef, error) {
	// make sure we were actually initalized
	if id.FilesystemCtx == nil {
		return nil, fserrors.FSCtxNotInitalized
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
func (id *File) Step(name string) (meta.WalkRef, error) {
	qid, err := id.QID()
	if err != nil {
		return nil, err
	}

	if qid.Type != p9.TypeDir {
		return nil, fserrors.ENOTDIR
	}

	tLen := len(id.Trail)
	id.Trail = append(id.Trail[:tLen:tLen], name)
	return id, nil
}

func (id *File) QID() (p9.QID, error) {
	if len(id.Trail) == 0 {
		return p9.QID{
			Type: p9.TypeDir,
			Path: meta.CidToQIDPath(meta.RootPath(id.CoreNamespace).Cid()),
		}, nil
	}

	callCtx, cancel := id.CallCtx()
	defer cancel()

	return meta.CoreToQID(callCtx, id.CorePath(), id.Core)
}

func (id *File) Backtrack() (meta.WalkRef, error) {
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
		return nil, fserrors.FSCtxNotInitalized
	}

	// and also not canceled / still valid
	if err := id.ParentCtx.Err(); err != nil {
		return nil, err
	}

	// all good; derive a new reference from this instance and return it
	return &File{
		CoreBase: id.CoreBase,
		OverlayBase: meta.OverlayBase{
			ParentCtx:     id.ParentCtx,
			FilesystemCtx: id.FilesystemCtx,
		},
		Parent: id.Parent,
	}, nil
}
