package mfs

import (
	"context"
	"errors"
	"io"
	"path/filepath"

	"github.com/billziss-gh/cgofuse/fuse"
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	files "github.com/ipfs/go-ipfs-files"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/mfs"
	logging "github.com/ipfs/go-log"
	gomfs "github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

var _ fuselib.FileSystemInterface = (*FileSystem)(nil)

type FileSystem struct {
	fusecom.SharedMethods
	provcom.IPFSCore

	initChan fusecom.InitSignal

	mroot       *gomfs.Root
	directories fusecom.DirectoryTable
	files       fusecom.FileTable

	log  logging.EventLogger
	ipfs fuselib.FileSystemInterface
}

func NewFileSystem(ctx context.Context, mroot gomfs.Root, core coreiface.CoreAPI, opts ...Option) *FileSystem {
	settings := parseOptions(opts...)

	return &FileSystem{
		IPFSCore: provcom.NewIPFSCore(ctx, core, settings.ResourceLock),
		initChan: settings.InitSignal,
		log:      settings.Log,
		mroot:    &mroot,
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
			c <- retErr
			close(fs.initChan)
		}

		fs.log.Errorf("init finished")
	}()

	// fs.mountTime = fuselib.Now()
	fs.directories = fusecom.NewDirectoryTable()
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
	fusecom.ApplyPermissions(true, &stat.Mode)
	stat.Uid, stat.Gid, _ = fuselib.Getcontext()
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

	goErr, errNo := fusecom.FillDir(directory, false, fill, ofst)
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

	file, err := mfs.OpenFile(fs.mroot, path, transform.IOFlagsFromFuse(flags))
	if err != nil {
		fs.log.Error(err)

		errNo := -fuselib.EIO
		var ioErr *transform.IOError
		if errors.As(err, &ioErr) {
			errNo = ioErr.ToFuse()
		}
		return errNo, fusecom.ErrorHandle
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

	// TODO: [review] we need to do things on failure
	// the OS typically triggers a close, but we shouldn't expect it to invalidate this record for us
	// we also might want to store a file cursor to reduce calls to seek
	// the same thing already happens internally so it's at worst the overhead of a call right now

	file, err := fs.files.Get(fh)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	if ofst < 0 {
		fs.log.Errorf("Read - Invalid offset {%d}[%X]%q", ofst, fh, path)
		return -fuselib.EINVAL
	}

	if fileBound, err := file.Size(); err == nil {
		if ofst >= fileBound {
			return 0 // POSIX expects this
		}
	}

	if ofst != 0 {
		_, err := file.Seek(ofst, io.SeekStart)
		if err != nil {
			fs.log.Errorf("Read - seek error: %s", err)
			return -fuselib.EIO
		}
	}

	retVal, err := file.Read(buff)
	if err != nil && err != io.EOF {
		fs.log.Errorf("Read - error: %s", err)
		return -fuselib.EIO
	}
	return retVal
}

func (fs *FileSystem) Readlink(path string) (int, string) {
	fs.log.Debugf("Readlink - %q", path)

	switch path {
	case "/":
		fs.log.Warnf("Readlink - root path is an invalid request")
		return -fuse.EINVAL, ""

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

	// TODO: timeout contexts
	corePath := corepath.IpldPath(ipldNode.Cid())
	iStat, _, err := transform.GetAttr(fs.Ctx(), corePath, fs.Core(), transform.IPFSStatRequest{Type: true})
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT, ""
	}

	if iStat.FileType != coreiface.TSymlink {
		fs.log.Errorf("Readlink - {%s}%q is not a symlink", iStat.FileType, path)
		return -fuse.EINVAL, ""
	}

	linkNode, err := fs.Core().Unixfs().Get(fs.Ctx(), corePath)
	if err != nil {
		fs.log.Error(err)
		return -fuse.EIO, ""
	}

	// NOTE: the implementation of this does no type checks
	// which is why we check the node's type above
	linkActual := files.ToSymlink(linkNode)

	// NOTE: paths returned here get sent back to the FUSE library
	// they should not be native paths
	return fusecom.OperationSuccess, filepath.ToSlash(linkActual.Target)
}
