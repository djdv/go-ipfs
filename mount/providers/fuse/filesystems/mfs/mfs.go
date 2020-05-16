package mfs

import (
	"context"
	"io"
	"path/filepath"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	coretransform "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/ipfscore"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/mfs"
	logging "github.com/ipfs/go-log"
	gomfs "github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

var _ fuselib.FileSystemInterface = (*FileSystem)(nil)

const filesWritable = true

type FileSystem struct {
	fusecom.SharedMethods
	provcom.IPFSCore

	initChan fusecom.InitSignal

	mroot       *gomfs.Root
	directories fusecom.DirectoryTable
	files       fusecom.FileTable

	log logging.EventLogger

	mountTimeGroup fusecom.StatTimeGroup
}

func NewFileSystem(ctx context.Context, mroot gomfs.Root, core coreiface.CoreAPI, opts ...Option) *FileSystem {
	settings := parseOptions(opts...)

	return &FileSystem{
		IPFSCore:    provcom.NewIPFSCore(ctx, core, settings.ResourceLock),
		initChan:    settings.InitSignal,
		log:         settings.Log,
		mroot:       &mroot,
		directories: fusecom.NewDirectoryTable(),
		files:       fusecom.NewFileTable(),
	}
}

func (fs *FileSystem) Init() {
	fs.Lock()
	fs.log.Debug("init")
	var retErr error
	defer func() {
		fs.Unlock()
		if retErr != nil {
			fs.log.Errorf("init failed: %s", retErr)
		}

		if c := fs.initChan; c != nil {
			if retErr != nil {
				c <- retErr
			}
			close(c)
		}

		fs.log.Debugf("init finished")
	}()

	// fs.mountTime = fuselib.Now()
	fs.directories = fusecom.NewDirectoryTable()
	fs.files = fusecom.NewFileTable()
	timeOfMount := fuselib.Now()
	fs.mountTimeGroup = fusecom.StatTimeGroup{
		Atim:     timeOfMount,
		Mtim:     timeOfMount,
		Ctim:     timeOfMount,
		Birthtim: timeOfMount,
	}
}

func (fs *FileSystem) Destroy() {
	fs.log.Debugf("Destroy - Requested")
	fs.mroot.Close()
}

func (fs *FileSystem) Getattr(path string, stat *fuselib.Stat_t, fh uint64) (errc int) {
	fs.log.Debugf("Getattr - {%X}%q", fh, path)

	mfsNode, err := gomfs.Lookup(fs.mroot, path)
	if err != nil {
		return -fuselib.ENOENT
	}

	ipldNode, err := mfsNode.GetNode()
	if err != nil {
		return -fuselib.EIO
	}

	corePath := corepath.IpldPath(ipldNode.Cid())

	iStat, _, err := transform.GetAttr(fs.Ctx(), corePath, fs.Core(), transform.IPFSStatRequestAll)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EIO
	}

	*stat = *iStat.ToFuse()
	var ids fusecom.StatIDGroup
	ids.Uid, ids.Gid, _ = fuselib.Getcontext()
	fusecom.ApplyCommonsToStat(stat, filesWritable, fs.mountTimeGroup, ids)
	return fusecom.OperationSuccess
}

func (fs *FileSystem) Opendir(path string) (int, uint64) {
	fs.log.Debugf("Opendir - %q", path)

	directory, err := mfs.OpenDir(fs.Ctx(), fs.mroot, path, fs.Core())
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}

	handle, err := fs.directories.Add(directory)
	if err != nil { // TODO: transform error
		fs.log.Error(fuselib.Error(-fuselib.EMFILE))
		return -fuselib.EMFILE, fusecom.ErrorHandle
	}

	return fusecom.OperationSuccess, handle
}

func (fs *FileSystem) Releasedir(path string, fh uint64) int {
	fs.log.Debugf("Releasedir - {%X}%q", fh, path)

	goErr, errNo := fusecom.ReleaseDir(fs.directories, fh)
	if goErr != nil {
		fs.log.Error(goErr)
	}

	return errNo
}

func (fs *FileSystem) Readdir(path string,
	fill func(name string, stat *fuselib.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) int {
	fs.log.Debugf("Readdir - Request {%X|%d}%q", fh, ofst, path)

	directory, err := fs.directories.Get(fh)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	goErr, errNo := fusecom.FillDir(fs.Ctx(), directory, fill, ofst)
	if goErr != nil {
		fs.log.Error(goErr)
	}

	return errNo
}

func (fs *FileSystem) Open(path string, flags int) (int, uint64) {
	fs.Lock()
	defer fs.Unlock()
	fs.log.Debugf("Open - {%X}%q", flags, path)

	goErr, errNo := fusecom.CheckOpenFlagsBasic(true, flags)
	if goErr != nil {
		fs.log.Error(goErr)
		return errNo, fusecom.ErrorHandle
	}

	goErr, errNo = fusecom.CheckOpenPathBasic(path)
	if goErr != nil {
		fs.log.Error(goErr)
		return errNo, fusecom.ErrorHandle
	}

	// TODO: rename back to err when other errors are abstracted
	file, tErr := mfs.OpenFile(fs.mroot, path, transform.IOFlagsFromFuse(flags))
	if tErr != nil {
		fs.log.Error(tErr)
		return tErr.ToFuse(), fusecom.ErrorHandle
	}

	handle, err := fs.files.Add(file)
	if err != nil { // TODO: transform error
		fs.log.Error(fuselib.Error(-fuselib.EMFILE))
		return -fuselib.EMFILE, fusecom.ErrorHandle
	}

	return fusecom.OperationSuccess, handle
}

func (fs *FileSystem) Release(path string, fh uint64) int {
	fs.log.Debugf("Release - {%X}%q", fh, path)

	goErr, errNo := fusecom.ReleaseFile(fs.files, fh)
	if goErr != nil {
		fs.log.Error(goErr)
	}

	return errNo
}

func (fs *FileSystem) Read(path string, buff []byte, ofst int64, fh uint64) int {
	fs.log.Debugf("Read - Request {%X|%d}%q", fh, ofst, path)

	file, err := fs.files.Get(fh)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	err, retVal := fusecom.ReadFile(file, buff, ofst)
	if err != nil && err != io.EOF {
		fs.log.Error(err)
	}
	return retVal
}

func (fs *FileSystem) Readlink(path string) (int, string) {
	fs.log.Debugf("Readlink - %q", path)

	// TODO: have something for this in fusecommon
	switch path {
	case "/":
		fs.log.Warnf("Readlink - root path is an invalid request")
		return -fuselib.EINVAL, ""

	case "":
		fs.log.Error("Readlink - empty request")
		return -fuselib.ENOENT, ""
	}

	mfsNode, err := gomfs.Lookup(fs.mroot, path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT, ""
	}

	ipldNode, err := mfsNode.GetNode()
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EIO, ""
	}

	linkString, tErr := coretransform.Readlink(fs.Ctx(), corepath.IpldPath(ipldNode.Cid()), fs.Core())
	if err != nil {
		fs.log.Error(tErr)
		return tErr.ToFuse(), ""
	}

	// NOTE: paths returned here get sent back to the FUSE library
	// they should not be native paths, regardless of their source format
	return fusecom.OperationSuccess, filepath.ToSlash(linkString)
}

func (fs *FileSystem) Create(path string, flags int, mode uint32) (int, uint64) {
	fs.log.Warnf("Create - {%X|%X}%q", flags, mode, path)

	// TODO: why is fuselib passing us flags and what are they?
	// both FUSE and SUS predefine what they should be (to Open)

	//return fs.open(path, fuselib.O_WRONLY|fuselib.O_CREAT|fuselib.O_TRUNC)

	// disabled until we parse relevant flags in open
	// fuse will do shenanigans to make this work
	return -fuselib.ENOSYS, fusecom.ErrorHandle
}

func (fs *FileSystem) Mknod(path string, mode uint32, dev uint64) int {
	fs.log.Debugf("Mknod - Request {%X|%d}%q", mode, dev, path)
	if err := mfs.Mknod(fs.mroot, path); err != nil {
		fs.log.Error(err)
		return err.ToFuse()
	}
	return fusecom.OperationSuccess
}

func (fs *FileSystem) Truncate(path string, size int64, fh uint64) int {
	fs.log.Warnf("Truncate - Request {%X|%d}%q", fh, size, path)

	if size < 0 {
		return -fuselib.EINVAL
	}

	if tf, err := fs.files.Get(fh); err == nil { // short path
		if err := tf.Truncate(uint64(size)); err != nil {
			fs.log.Error(err)
			return -fuselib.EIO
		}
		return fusecom.OperationSuccess
	}

	var loopPrevention bool
lookup:

	if node, err := gomfs.Lookup(fs.mroot, path); err == nil { // medium path
		file, ok := node.(*gomfs.File)
		if !ok {
			fs.log.Error(fuselib.Error(-fuselib.EISDIR))
			return -fuselib.EISDIR
		}
		fd, err := file.Open(gomfs.Flags{Write: true, Sync: true})
		if err != nil {
			fs.log.Error(err)
			return -fuselib.EIO
		}
		if err := fd.Truncate(size); err != nil {
			fs.log.Error(err)
			return -fuselib.EIO
		}
	} else if loopPrevention {
		fs.log.Error(err)
		return -fuselib.EIO
	}

	// long path
	// mknod; goto medium path
	if err := mfs.Mknod(fs.mroot, path); err != nil {
		fs.log.Error(err)
		return err.ToFuse()
	}
	loopPrevention = true
	goto lookup
}

func (fs *FileSystem) Write(path string, buff []byte, ofst int64, fh uint64) int {
	fs.log.Warnf("Write - Request {%X|%d|%d}%q", fh, len(buff), ofst, path)
	file, err := fs.files.Get(fh)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	err, retVal := fusecom.WriteFile(file, buff, ofst)
	if err != nil && err != io.EOF {
		fs.log.Error(err)
	}
	return retVal
}

func (fs *FileSystem) Link(oldpath string, newpath string) int {
	fs.log.Warnf("Link - Request %q<->%q", oldpath, newpath)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Unlink(path string) int {
	fs.log.Debugf("Unlink - Request %q", path)

	if err := mfs.Unlink(fs.mroot, path); err != nil {
		fs.log.Error(err)
		return err.ToFuse()
	}

	return fusecom.OperationSuccess
}

func (fs *FileSystem) Mkdir(path string, mode uint32) int {
	fs.log.Debugf("Mkdir - Request {%X}%q", mode, path)
	if err := mfs.Mkdir(fs.mroot, path); err != nil {
		fs.log.Error(err)
		return err.ToFuse()
	}
	return fusecom.OperationSuccess
}

func (fs *FileSystem) Rmdir(path string) int {
	fs.log.Warnf("Rmdir - Request %q", path)

	if err := mfs.Rmdir(fs.mroot, path); err != nil {
		fs.log.Error(err)
		return err.ToFuse()
	}

	return fusecom.OperationSuccess
}

func (fs *FileSystem) Symlink(target string, newpath string) int {
	fs.log.Debugf("Symlink - Request %q->%q", newpath, target)

	if err := mfs.Symlink(fs.mroot, newpath, target); err != nil {
		fs.log.Error(err)
		return err.ToFuse()
	}

	return fusecom.OperationSuccess
}

// TODO: error disambiguation
// TODO: account for open handles (fun)
func (fs *FileSystem) Rename(oldpath string, newpath string) int {
	fs.log.Warnf("Rename - Request %q->%q", oldpath, newpath)

	if err := gomfs.Mv(fs.mroot, oldpath, newpath); err != nil {
		fs.log.Error(err)
		return -fuselib.EIO
	}

	return fusecom.OperationSuccess
}
