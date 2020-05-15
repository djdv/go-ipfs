// Package mfs exposes the IPFS Mutable File System API as a 9P compatible resource server
package mfs

import (
	"context"
	"fmt"
	"io"
	gopath "path"
	"sync/atomic"

	"github.com/hugelgupf/p9/fsimpl/templatefs"
	"github.com/hugelgupf/p9/p9"
	common "github.com/ipfs/go-ipfs/mount/providers/9P/filesystems"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-mfs"
	"github.com/ipfs/go-unixfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var _ p9.File = (*File)(nil)
var _ common.WalkRef = (*File)(nil)

type File struct {
	templatefs.NoopFile
	p9.DefaultWalkGetAttr

	common.CoreBase
	common.OverlayBase

	openFlags p9.OpenFlags //TODO: move this to IPFSBase; use as open marker
	file      *mfs.File
	directory *mfs.Directory

	mroot  *mfs.Root
	parent common.WalkRef
	open   bool
}

func Attacher(ctx context.Context, core coreiface.CoreAPI, ops ...common.AttachOption) p9.Attacher {
	options := common.AttachOps(ops...)

	if options.MFSRoot == nil {
		panic("MFS root is required but was not defined in provided options")
	}

	md := &File{
		CoreBase: common.NewCoreBase("/ipld", core, ops...),
		OverlayBase: common.OverlayBase{
			ParentCtx: ctx,
			Opened:    new(uintptr),
		},
		mroot:  options.MFSRoot,
		parent: options.Parent,
	}

	return md
}

func (md *File) Fork() (common.WalkRef, error) {
	newFid, err := md.clone()
	if err != nil {
		md.Logger.Error(err)
		return nil, err
	}

	// make sure we were actually initialized
	if md.FilesystemCtx == nil {
		md.Logger.Error(common.FSCtxNotInitialized)
		return nil, common.FSCtxNotInitialized
	}

	// and also not canceled / still valid
	if err := md.FilesystemCtx.Err(); err != nil {
		md.Logger.Error(err)
		return nil, err
	}

	newFid.OperationsCtx, newFid.OperationsCancel = context.WithCancel(md.FilesystemCtx)

	return newFid, nil
}

func (md *File) Attach() (p9.File, error) {
	md.Logger.Debugf("Attach")

	newFid, err := md.clone()
	if err != nil {
		md.Logger.Error(err)
		return nil, err
	}

	newFid.FilesystemCtx, newFid.FilesystemCancel = context.WithCancel(newFid.ParentCtx)
	return newFid, nil
}

func (md *File) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	md.Logger.Debugf("GetAttr path: %s", md.StringPath())
	md.Logger.Debugf("%p", md)

	qid, err := md.QID()
	if err != nil {
		md.Logger.Error(err)
		return qid, p9.AttrMask{}, p9.Attr{}, err
	}

	attr, filled, err := md.getAttr(req)
	if err != nil {
		md.Logger.Error(err)
		return qid, filled, attr, err
	}

	if req.RDev {
		attr.RDev, filled.RDev = common.DevMemory, true
	}

	if req.Mode {
		attr.Mode |= common.IRXA | 0220
	}

	return qid, filled, attr, nil
}

func (md *File) Walk(names []string) ([]p9.QID, p9.File, error) {
	md.Logger.Debugf("Walk %q: %v", md.String(), names)
	return common.Walker(md, names)
}

func (md *File) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	md.Logger.Debugf("Open %q {Mode:%v OSFlags:%v, String:%s}", md.StringPath(), mode.Mode(), mode.OSFlags(), mode.String())
	md.Logger.Debugf("%p", md)

	if md.IsOpen() {
		md.Logger.Error(common.FileOpen)
		return p9.QID{}, 0, common.FileOpen
	}

	qid, err := md.QID()
	if err != nil {
		md.Logger.Error(err)
		return p9.QID{}, 0, err
	}

	attr, _, err := md.getAttr(p9.AttrMask{Mode: true})
	if err != nil {
		md.Logger.Error(err)
		return qid, 0, err
	}

	switch {
	case attr.Mode.IsDir():
		dir, err := md.getDirectory()
		if err != nil {
			md.Logger.Error(err)
			return qid, 0, err
		}

		md.directory = dir

	case attr.Mode.IsRegular():
		mFile, err := md.getFile()
		if err != nil {
			md.Logger.Error(err)
			return qid, 0, err
		}

		md.file = mFile
	}

	atomic.StoreUintptr(md.Opened, 1)
	md.openFlags = mode // TODO: convert to MFS native flags
	md.open = true
	return qid, common.UFS1BlockSize, nil
}

func (md *File) Readdir(offset uint64, count uint32) (p9.Dirents, error) {
	md.Logger.Debugf("Readdir %d %d", offset, count)

	if md.directory == nil {
		return nil, fmt.Errorf("directory %q is not open for reading", md.StringPath())
	}

	//TODO: resetable context; for { ...; ctx.reset() }
	callCtx, cancel := context.WithCancel(md.FilesystemCtx)
	defer cancel()

	ents := make(p9.Dirents, 0)

	var index uint64
	var done bool
	err := md.directory.ForEachEntry(callCtx, func(nl mfs.NodeListing) error {
		if done {
			cancel()
			return nil
		}

		if index < offset {
			index++ //skip
			return nil
		}

		ent, err := common.MFSEntTo9Ent(nl)
		if err != nil {
			md.Logger.Error(err)
			return err
		}

		ent.Offset = index + 1

		ents = append(ents, ent)
		if len(ents) == int(count) {
			done = true
		}

		index++
		return nil
	})

	return ents, err
}

func (md *File) ReadAt(p []byte, offset int64) (int, error) {
	const readAtFmtErr = "ReadAt {%d}%q: %s"

	if md.file == nil {
		err := fmt.Errorf("file is not open for reading")
		md.Logger.Errorf(readAtFmtErr, offset, md.StringPath(), err)
		return 0, err
	}

	attr, _, err := md.getAttr(p9.AttrMask{Size: true})
	if err != nil {
		md.Logger.Error(err)
		return 0, err
	}

	// FIXME: unsafe compare; maxsize
	if uint64(offset) >= attr.Size {
		//NOTE [styx]: If the offset field is greater than or equal to the number of bytes in the file, a count of zero will be returned.
		return 0, io.EOF
	}

	openFile, err := md.file.Open(mfs.Flags{Read: true})
	if err != nil {
		md.Logger.Error(err)
		return 0, err
	}
	defer openFile.Close()

	if _, err := openFile.Seek(int64(offset), io.SeekStart); err != nil {
		md.Logger.Errorf(readAtFmtErr, offset, md.StringPath(), err)
		return 0, err
	}

	return openFile.Read(p)
}

func (md *File) SetAttr(valid p9.SetAttrMask, attr p9.SetAttr) error {
	md.Logger.Debugf("SetAttr %v %v", valid, attr)
	md.Logger.Debugf("%p", md)

	if valid.Size {
		var target *mfs.File

		if md.file != nil {
			target = md.file
		} else {
			mFile, err := md.getFile()
			if err != nil {
				return err
			}

			target = mFile
		}

		openFile, err := target.Open(mfs.Flags{Read: true, Write: true})
		if err != nil {
			md.Logger.Error(err)
			return err
		}
		defer openFile.Close()

		if err := openFile.Truncate(int64(attr.Size)); err != nil {
			md.Logger.Error(err)
			return err
		}
	}

	// TODO: requires a form of metadata storage (like UFSv2)
	// md.common.Apply(valid, attr)
	return nil
}

func (md *File) WriteAt(p []byte, offset int64) (int, error) {
	const readAtFmtErr = "WriteAt {%d}%q: %s"

	if md.file == nil {
		err := fmt.Errorf("file is not open for writing")
		md.Logger.Errorf(readAtFmtErr, offset, md.StringPath(), err)
		return 0, err
	}

	openFile, err := md.file.Open(mfs.Flags{Read: true, Write: true})
	if err != nil {
		md.Logger.Error(err)
		return 0, err
	}
	defer openFile.Close()

	nbytes, err := openFile.WriteAt(p, offset)
	if err != nil {
		md.Logger.Errorf(readAtFmtErr, offset, md.StringPath(), err)
		return nbytes, err
	}

	if err = openFile.Flush(); err != nil {
		md.Logger.Error(err)
		return nbytes, err
	}

	return nbytes, nil

	//return md.file.WriteAt(p, int64(offset))
}

func (md *File) Close() error {
	md.Closed = true
	if md.open {
		atomic.StoreUintptr(md.Opened, 0)
	}

	md.file = nil
	md.directory = nil
	if md.mroot != nil {
		return md.mroot.Flush()
	}
	return nil
}

/*
{
    Base: {
	coreNamespace: `/ipld`,
	Trail: []string{"folder", "file.txt"}
    }
    mroot: fromCid(`QmVuDpaFj55JnUH7UYxTAydx6ayrs2cB3Gb7cdPr61wLv5`)
}
=>
`/ipld/QmVuDpaFj55JnUH7UYxTAydx6ayrs2cB3Gb7cdPr61wLv5/folder/file.txt`
*/
func (md *File) StringPath() string {
	rootNode, err := md.mroot.GetDirectory().GetNode()
	if err != nil {
		panic(err)
	}
	return gopath.Join(append([]string{md.CoreNamespace, rootNode.Cid().String()}, md.Trail...)...)
}

func (md *File) Step(name string) (common.WalkRef, error) {

	// FIXME: [in general] Step should return ref, qid, error
	// obviate CheckWalk + QID and make this implicit via Step
	qid, err := md.QID()
	if err != nil {
		md.Logger.Error(err)
		return nil, err
	}

	if qid.Type != p9.TypeDir {
		md.Logger.Error(common.ENOTDIR)
		return nil, common.ENOTDIR
	}

	tLen := len(md.Trail)
	md.Trail = append(md.Trail[:tLen:tLen], name)
	return md, nil
}

/*
func (md *MFS) RootPath(keyName string, components ...string) (corepath.Path, error) {
	if keyName == "" {
		return nil, fmt.Errorf("no path key was provided")
	}

	rootCid, err := cid.Decode(keyName)
	if err != nil {
		return nil, err
	}

	return corepath.Join(corepath.IpldPath(rootCid), components...), nil
}

func (md *MFS) ResolvedPath(names ...string) (corepath.Path, error) {
	callCtx, cancel := md.CallCtx()
	defer cancel()

	return md.core.ResolvePath(callCtx, md.KeyPath(names[0], names[1:]...))

	corePath = corepath.IpldPath(md.Tail[0])
	return md.core.ResolvePath(callCtx, corepath.Join(corePath, append(md.Tail[1:], names)...))
}
*/

func (md *File) Backtrack() (common.WalkRef, error) {
	if md.parent != nil {
		return md.parent, nil
	}
	return md, nil
}

func (md *File) QID() (p9.QID, error) {
	mNode, err := mfs.Lookup(md.mroot, gopath.Join(md.Trail...))
	if err != nil {
		md.Logger.Error(err)
		return p9.QID{}, err
	}

	t, err := common.MFSTypeToNineType(mNode.Type())
	if err != nil {
		md.Logger.Error(err)
		return p9.QID{}, err
	}

	ipldNode, err := mNode.GetNode()
	if err != nil {
		md.Logger.Error(err)
		return p9.QID{}, err
	}

	return p9.QID{
		Type: t,
		Path: common.CidToQIDPath(ipldNode.Cid()),
	}, nil
}

func (md *File) getNode() (ipld.Node, error) {
	mNode, err := mfs.Lookup(md.mroot, gopath.Join(md.Trail...))
	if err != nil {
		return nil, err
	}
	return mNode.GetNode()
}

func (md *File) getFile() (*mfs.File, error) {
	mNode, err := mfs.Lookup(md.mroot, gopath.Join(md.Trail...))
	if err != nil {
		return nil, err
	}

	mFile, ok := mNode.(*mfs.File)
	if !ok {
		return nil, fmt.Errorf("type mismatch %q is %T not a file", md.StringPath(), mNode)
	}

	return mFile, nil
}

func (md *File) getDirectory() (*mfs.Directory, error) {
	mNode, err := mfs.Lookup(md.mroot, gopath.Join(md.Trail...))
	if err != nil {
		return nil, err
	}

	dir, ok := mNode.(*mfs.Directory)
	if !ok {
		return nil, fmt.Errorf("type mismatch %q is %T not a directory", md.StringPath(), mNode)
	}
	return dir, nil
}

func (md *File) getAttr(req p9.AttrMask) (p9.Attr, p9.AttrMask, error) {
	var attr p9.Attr

	mfsNode, err := mfs.Lookup(md.mroot, gopath.Join(md.Trail...))
	if err != nil {
		return attr, p9.AttrMask{}, err
	}

	ipldNode, err := mfsNode.GetNode()
	if err != nil {
		return attr, p9.AttrMask{}, err
	}

	callCtx, cancel := md.CallCtx()
	defer cancel()

	filled, err := common.IpldStat(callCtx, &attr, ipldNode, req)
	if err != nil {
		md.Logger.Error(err)
	}
	return attr, filled, err
}

func (md *File) Create(name string, flags p9.OpenFlags, permissions p9.FileMode, uid p9.UID, gid p9.GID) (p9.File, p9.QID, uint32, error) {
	callCtx, cancel := md.CallCtx()
	defer cancel()

	emptyNode, err := emptyNode(callCtx, md.Core.Dag())
	if err != nil {
		md.Logger.Error(err)
		return nil, p9.QID{}, 0, err
	}

	err = mfs.PutNode(md.mroot, gopath.Join(append(md.Trail, name)...), emptyNode)
	if err != nil {
		md.Logger.Error(err)
		return nil, p9.QID{}, 0, err
	}

	newFid, err := md.Fork()
	if err != nil {
		md.Logger.Error(err)
		return nil, p9.QID{}, 0, err
	}

	newRef, err := newFid.Step(name)
	if err != nil {
		md.Logger.Error(err)
		return nil, p9.QID{}, 0, err
	}

	qid, ioUnit, err := newRef.Open(flags)
	return newRef, qid, ioUnit, err
}

func emptyNode(ctx context.Context, dagAPI coreiface.APIDagService) (ipld.Node, error) {
	eFile := dag.NodeWithData(unixfs.FilePBData(nil, 0))
	if err := dagAPI.Add(ctx, eFile); err != nil {
		return nil, err
	}
	return eFile, nil
}

func (md *File) Mkdir(name string, permissions p9.FileMode, uid p9.UID, gid p9.GID) (p9.QID, error) {
	err := mfs.Mkdir(md.mroot, gopath.Join(append(md.Trail, name)...), mfs.MkdirOpts{Flush: true})
	if err != nil {
		md.Logger.Error(err)
		return p9.QID{}, err
	}

	newFid, err := md.Fork()
	if err != nil {
		md.Logger.Error(err)
		return p9.QID{}, err
	}
	newRef, err := newFid.Step(name)
	if err != nil {
		md.Logger.Error(err)
		return p9.QID{}, err
	}

	return newRef.QID()
}

func (md *File) parentDir() (*mfs.Directory, error) {
	parent := gopath.Dir(gopath.Join(md.Trail...))

	mNode, err := mfs.Lookup(md.mroot, parent)
	if err != nil {
		return nil, err
	}

	dir, ok := mNode.(*mfs.Directory)
	if !ok {
		return nil, fmt.Errorf("type mismatch %q is %T not a directory", md.StringPath(), mNode)
	}
	return dir, nil
}

func (md *File) Mknod(name string, mode p9.FileMode, major uint32, minor uint32, uid p9.UID, gid p9.GID) (p9.QID, error) {
	callCtx, cancel := md.CallCtx()
	defer cancel()

	emptyNode, err := emptyNode(callCtx, md.Core.Dag())
	if err != nil {
		md.Logger.Error(err)
		return p9.QID{}, err
	}

	err = mfs.PutNode(md.mroot, gopath.Join(append(md.Trail, name)...), emptyNode)
	if err != nil {
		md.Logger.Error(err)
		return p9.QID{}, err
	}

	newFid, err := md.Fork()
	if err != nil {
		md.Logger.Error(err)
		return p9.QID{}, err
	}
	newRef, err := newFid.Step(name)
	if err != nil {
		md.Logger.Error(err)
		return p9.QID{}, err
	}

	return newRef.QID()
}

func (md *File) UnlinkAt(name string, flags uint32) error {
	dir, err := md.getDirectory()
	if err != nil {
		md.Logger.Error(err)
		return err
	}
	return dir.Unlink(name)
}

func (md *File) clone() (*File, error) {
	// make sure we were actually initialized
	if md.ParentCtx == nil {
		return nil, common.FSCtxNotInitialized
	}

	// and also not canceled / still valid
	if err := md.ParentCtx.Err(); err != nil {
		return nil, err
	}

	// all good; derive a new reference from this instance and return it
	return &File{
		CoreBase:    md.CoreBase,
		OverlayBase: md.OverlayBase.Clone(),
		parent:      md.parent,
		mroot:       md.mroot,
	}, nil
}
