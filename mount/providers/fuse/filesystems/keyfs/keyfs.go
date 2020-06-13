package keyfs

import (
	"context"
	"io"
	"path/filepath"
	"time"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/keyfs"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var _ fuselib.FileSystemInterface = (*FileSystem)(nil)

const filesWritable = true

// TODO: [general] abstract the core FS into a `Root`
// subsume subnodes for IPNS into our tables like was done with MFS
// i.e. subfiles are tracked via our file table, not an external one

type FileSystem struct {
	fusecom.SharedMethods

	initChan fusecom.InitSignal

	// maps keys to a single shared I/O instance for that key
	intf transform.Interface

	// maps handles to an I/O wrapper that looks unique but uses the same underlying I/O
	files       fusecom.FileTable
	directories fusecom.DirectoryTable

	log logging.EventLogger

	mountTimeGroup fusecom.StatTimeGroup
	rootStat       *fuselib.Stat_t
}

func NewFileSystem(ctx context.Context, core coreiface.CoreAPI, opts ...Option) *FileSystem {
	settings := parseOptions(opts...)

	return &FileSystem{
		intf:     keyfs.NewInterface(ctx, core),
		initChan: settings.InitSignal,
		log:      settings.Log,
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

	fs.files = fusecom.NewFileTable()
	fs.directories = fusecom.NewDirectoryTable()

	timeOfMount := fuselib.Now()

	fs.mountTimeGroup = fusecom.StatTimeGroup{
		Atim:     timeOfMount,
		Mtim:     timeOfMount,
		Ctim:     timeOfMount,
		Birthtim: timeOfMount,
	}

	fs.rootStat = &fuselib.Stat_t{
		Mode:     fuselib.S_IFDIR | fusecom.IRWXA&^(fuselib.S_IWOTH|fuselib.S_IXOTH), // |0774
		Atim:     timeOfMount,
		Mtim:     timeOfMount,
		Ctim:     timeOfMount,
		Birthtim: timeOfMount,
	}
}

func (fs *FileSystem) Destroy() {
	fs.log.Debugf("Destroy - Requested")
}

func (fs *FileSystem) Getattr(path string, stat *fuselib.Stat_t, fh uint64) (errc int) {
	fs.log.Debugf("Getattr - {%X}%q", fh, path)

	if path == "/" { // root request
		*stat = *fs.rootStat
		stat.Uid, stat.Gid, _ = fuselib.Getcontext()
		return fusecom.OperationSuccess
	}

	iStat, _, err := fs.intf.Info(path, transform.IPFSStatRequestAll)
	if err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err)
	}

	// TODO: we need a way to distinguish between IPNS and MFS requests
	// so that we don't flag IPNS files as writable
	var ids fusecom.StatIDGroup
	ids.Uid, ids.Gid, _ = fuselib.Getcontext()
	fusecom.ApplyIntermediateStat(stat, iStat)
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
		fs.log.Error(err)
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
	fs.log.Debugf("Readdir - {%X|%d}%q", fh, ofst, path)

	directory, err := fs.directories.Get(fh)
	if err != nil {
		fs.log.Error(err)
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
	return fs.open(path, flags)
}

func (fs *FileSystem) open(path string, flags int) (int, uint64) {
	errNo, err := fusecom.CheckOpenFlagsBasic(filesWritable, flags)
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
		fs.log.Error(err)
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
	fs.log.Debugf("Read - {%X}%q", fh, path)

	if path == "/" { // root request; we're never a file
		fs.log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

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

	if path == "/" {
		return -fuselib.EINVAL, ""
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
	fs.log.Debugf("Truncate - Request {%X|%d}%q", fh, size, path)

	if size < 0 {
		return -fuselib.EINVAL
	}

	var didOpen bool
	file, err := fs.files.Get(fh) // use the handle if it's valid
	if err != nil {               // otherwise fallback to open
		file, err = fs.intf.Open(path, transform.IOWriteOnly)
		if err != nil {
			fs.log.Error(err)
			return fusecom.InterpretError(err)
		}
		didOpen = true
	}

	if err = file.Truncate(uint64(size)); err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err)
	}

	if didOpen {
		if err := file.Close(); err != nil {
			fs.log.Error(err)
			return fusecom.InterpretError(err)
		}
	}

	return fusecom.OperationSuccess
}

func (fs *FileSystem) Write(path string, buff []byte, ofst int64, fh uint64) int {
	fs.log.Debugf("Write - Request {%X|%d|%d}%q", fh, len(buff), ofst, path)

	if path == "/" { // root request; we're never a file
		fs.log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

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

	if path == "/" {
		fs.log.Error(fuselib.Error(-fuselib.EPERM))
		return -fuselib.EPERM
	}

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
	fs.log.Debugf("Rmdir - Request %q", path)

	if err := fs.intf.RemoveDirectory(path); err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err)
	}

	return fusecom.OperationSuccess
}

func (fs *FileSystem) Symlink(target string, newpath string) int {
	fs.log.Debugf("Symlink - Request %q->%q", newpath, target)

	if err := fs.intf.MakeLink(target, newpath); err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err)
	}

	return fusecom.OperationSuccess
}

// TODO: account for open handles (fun)
// TODO: cross FS moves (also fun) (for now, hacky key:key and mfs:mfs only, no cross talk yet, bad checking scheme)
func (fs *FileSystem) Rename(oldpath string, newpath string) int {
	fs.log.Warnf("Rename - Request %q->%q", oldpath, newpath)

	if err := fs.intf.Rename(oldpath, newpath); err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err)
	}

	return fusecom.OperationSuccess
}
