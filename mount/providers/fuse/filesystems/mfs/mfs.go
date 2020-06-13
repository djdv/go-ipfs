package mfs

import (
	"context"
	"io"
	"path/filepath"
	"time"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/mfs"
	logging "github.com/ipfs/go-log"
	gomfs "github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var _ fuselib.FileSystemInterface = (*FileSystem)(nil)

const filesWritable = true

type FileSystem struct {
	fusecom.SharedMethods

	initChan fusecom.InitSignal

	intf        transform.Interface
	directories fusecom.DirectoryTable
	files       fusecom.FileTable

	log logging.EventLogger

	mountTimeGroup fusecom.StatTimeGroup
}

func NewFileSystem(ctx context.Context, mroot *gomfs.Root, core coreiface.CoreAPI, opts ...Option) *FileSystem {
	settings := parseOptions(opts...)

	return &FileSystem{
		initChan:    settings.InitSignal,
		log:         settings.Log,
		intf:        mfs.NewInterface(ctx, mroot),
		directories: fusecom.NewDirectoryTable(),
		files:       fusecom.NewFileTable(),
	}
}

func (fs *FileSystem) Init() {
	fs.log.Debug("init")
	var retErr error
	defer func() {
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
	fs.intf.Close()
}

func (fs *FileSystem) Getattr(path string, stat *fuselib.Stat_t, fh uint64) (errc int) {
	fs.log.Debugf("Getattr - {%X}%q", fh, path)

	iStat, _, err := fs.intf.Info(path, transform.IPFSStatRequestAll)
	if err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err)
	}

	*stat = *iStat.ToFuse()
	var ids fusecom.StatIDGroup
	ids.Uid, ids.Gid, _ = fuselib.Getcontext()
	fusecom.ApplyCommonsToStat(stat, filesWritable, fs.mountTimeGroup, ids)
	return fusecom.OperationSuccess
}

func (fs *FileSystem) Opendir(path string) (int, uint64) {
	fs.log.Debugf("Opendir - %q", path)

	directory, err := fs.intf.OpenDirectory(path)
	if err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err), fusecom.ErrorHandle
	}

	handle, err := fs.directories.Add(directory)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.EMFILE))
		return -fuselib.EMFILE, fusecom.ErrorHandle
	}

	return fusecom.OperationSuccess, handle
}

func (fs *FileSystem) Releasedir(path string, fh uint64) int {
	fs.log.Debugf("Releasedir - {%X}%q", fh, path)

	errNo, err := fusecom.ReleaseDir(fs.directories, fh)
	if err != nil {
		fs.log.Error(err)
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

	// TODO: change this context; needs parent
	callCtx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
	defer cancel()

	errNo, err := fusecom.FillDir(callCtx, directory, fill, ofst)
	if err != nil {
		fs.log.Error(err)
	}

	return errNo
}

func (fs *FileSystem) Open(path string, flags int) (int, uint64) {
	fs.log.Debugf("Open - {%X}%q", flags, path)

	errNo, err := fusecom.CheckOpenFlagsBasic(filesWritable, flags)
	if err != nil {
		fs.log.Error(err)
		return errNo, fusecom.ErrorHandle
	}

	errNo, err = fusecom.CheckOpenPathBasic(path)
	if err != nil {
		fs.log.Error(err)
		return errNo, fusecom.ErrorHandle
	}

	file, err := fs.intf.Open(path, fusecom.IOFlagsFromFuse(flags))
	if err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err), fusecom.ErrorHandle
	}

	handle, err := fs.files.Add(file)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.EMFILE))
		return -fuselib.EMFILE, fusecom.ErrorHandle
	}

	return fusecom.OperationSuccess, handle
}

func (fs *FileSystem) Release(path string, fh uint64) int {
	fs.log.Debugf("Release - {%X}%q", fh, path)

	errNo, err := fusecom.ReleaseFile(fs.files, fh)
	if err != nil {
		fs.log.Error(err)
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

	errNo, err := fusecom.ReadFile(file, buff, ofst)
	if err != nil && err != io.EOF {
		fs.log.Error(err)
	}
	return errNo
}

func (fs *FileSystem) Readlink(path string) (int, string) {
	fs.log.Debugf("Readlink - %q", path)

	// TODO: wrap path checks in fusecommon
	switch path {
	case "/":
		fs.log.Warnf("Readlink - root path is an invalid request")
		return -fuselib.EINVAL, ""

	case "":
		fs.log.Error("Readlink - empty request")
		return -fuselib.ENOENT, ""
	}

	linkString, err := fs.intf.ExtractLink(path)
	if err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err), ""
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

	if err := fs.intf.Make(path); err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err)
	}

	return fusecom.OperationSuccess
}

func (fs *FileSystem) Truncate(path string, size int64, fh uint64) int {
	fs.log.Warnf("Truncate - Request {%X|%d}%q", fh, size, path)

	if size < 0 {
		return -fuselib.EINVAL
	}

	tf, err := fs.files.Get(fh) // short path; handle to file
	if err == nil {
		err = tf.Truncate(uint64(size)) // truncate and return
		goto ret
	}

	// slow path; open the file for truncation
	tf, err = fs.intf.Open(path, transform.IOWriteOnly)
	if err != nil {
		goto ret
	}
	if err := tf.Truncate(uint64(size)); err != nil {
		goto ret
	}
	err = tf.Close()

ret:
	if err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err)
	}

	return fusecom.OperationSuccess
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

	if err := fs.intf.Remove(path); err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err)
	}

	return fusecom.OperationSuccess
}

func (fs *FileSystem) Mkdir(path string, mode uint32) int {
	fs.log.Debugf("Mkdir - Request {%X}%q", mode, path)

	if err := fs.intf.MakeDirectory(path); err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err)
	}

	return fusecom.OperationSuccess
}

func (fs *FileSystem) Rmdir(path string) int {
	fs.log.Warnf("Rmdir - Request %q", path)

	if err := fs.intf.RemoveDirectory(path); err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err)
	}

	return fusecom.OperationSuccess
}

func (fs *FileSystem) Symlink(target string, newpath string) int {
	fs.log.Debugf("Symlink - Request %q->%q", newpath, target)

	if err := fs.intf.MakeLink(newpath, target); err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err)
	}

	return fusecom.OperationSuccess
}

// TODO: account for open handles (fun)
func (fs *FileSystem) Rename(oldpath string, newpath string) int {
	fs.log.Warnf("Rename - Request %q->%q", oldpath, newpath)

	if err := fs.intf.Rename(oldpath, newpath); err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err)
	}

	return fusecom.OperationSuccess
}
