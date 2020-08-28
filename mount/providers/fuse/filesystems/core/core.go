package ipfscore

import (
	"context"
	"io"
	gopath "path"
	"strings"

	"github.com/billziss-gh/cgofuse/fuse"
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	files "github.com/ipfs/go-ipfs-files"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	mountcom "github.com/ipfs/go-ipfs/mount/utils/common"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/empty"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/ipfscore"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

var _ fuselib.FileSystemInterface = (*FileSystem)(nil)

type FileSystem struct {
	provcom.IPFSCore

	initChan fusecom.InitSignal

	files       fusecom.FileTable
	directories fusecom.DirectoryTable

	log       logging.EventLogger
	namespace mountinter.Namespace

	// TODO: remove this; packages should implement all interface methods within the package
	fuselib.FileSystemBase
}

func NewFileSystem(ctx context.Context, core coreiface.CoreAPI, opts ...Option) *FileSystem {
	options := new(options)
	for _, opt := range opts {
		opt.apply(options)
	}

	if options.resourceLock == nil {
		options.resourceLock = mountcom.NewResourceLocker()
	}

	if options.log != nil {
		options.log = logging.Logger("fuse/ipld")
	}

	if options.namespace == mountinter.NamespaceNone {
		options.namespace = mountinter.NamespaceCore
	}

	return &FileSystem{
		IPFSCore:    provcom.NewIPFSCore(ctx, core, options.resourceLock),
		initChan:    options.initSignal,
		files:       fusecom.NewFileTable(),
		directories: fusecom.NewDirectoryTable(),
		log:         options.log,
		namespace:   options.namespace,
	}
}

func (fs *FileSystem) Init() {
	fs.Lock()
	defer fs.Unlock()
	fs.log.Debug("init")

	/*
		fs.handles = make(fsHandles)
		fs.mountTime = fuselib.Now()
	*/

	defer fs.log.Debug("init finished")
	if c := fs.initChan; c != nil {
		c <- nil
	}
}

func (fs *FileSystem) Open(path string, flags int) (int, uint64) {
	fs.Lock()
	defer fs.Unlock()
	fs.log.Debugf("Open - {%X}%q", flags, path)

	// TODO: parse flags

	switch path {
	case "":
		// TODO: handle empty path (valid if fh is valid)
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT, fusecom.ErrorHandle
	case "/":
		fs.log.Error(fuselib.Error(-fuselib.EISDIR))
		return -fuselib.EISDIR, fusecom.ErrorHandle
	default:
		file, err := ipfscore.OpenFile(fs.Ctx(), fs.namespace, path[1:], fs.Core(), transform.IOFlagsFromFuse(flags))
		if err != nil {
			// TODO: proper error translations transError.ToFuse(), etc.
			// EIO might not be appropriate here either ref: POSIX open()
			fs.log.Error(fuselib.Error(-fuselib.EIO))
			return -fuselib.EIO, fusecom.ErrorHandle
		}

		handle, err := fs.files.Add(file)
		if err != nil { // TODO: transform error
			fs.log.Error(fuselib.Error(-fuselib.ENFILE))
			return -fuselib.ENFILE, fusecom.ErrorHandle
		}

		return fusecom.OperationSuccess, handle
	}
}

func (fs *FileSystem) Opendir(path string) (int, uint64) {
	fs.log.Debugf("Opendir - %q", path)
	var directory transform.Directory
	switch path {
	case "":
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT, fusecom.ErrorHandle

	case "/":
		directory = empty.OpenDir()

	default:
		coreDir, err := ipfscore.OpenDir(fs.Ctx(), fs.namespace, path[1:], fs.Core())
		if err != nil {
			fs.log.Error(err)
			return -fuselib.ENOENT, fusecom.ErrorHandle
		}
		directory = coreDir
	}

	handle, err := fs.directories.Add(directory)
	if err != nil { // TODO: transform error
		fs.log.Error(fuselib.Error(-fuselib.ENFILE))
		return -fuselib.ENFILE, fusecom.ErrorHandle
	}

	return fusecom.OperationSuccess, handle
}

func (fs *FileSystem) Releasedir(path string, fh uint64) int {
	fs.log.Debugf("Releasedir - {%X}%q", fh, path)

	if fh == fusecom.ErrorHandle {
		fs.log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	if err := fs.directories.Remove(fh); err != nil {
		fs.log.Error(err)
		return -fuselib.EBADF
	}
	return fusecom.OperationSuccess
}

func (fs *FileSystem) Release(path string, fh uint64) int {
	fs.log.Debugf("Release - {%X}%q", fh, path)

	if fh == fusecom.ErrorHandle {
		fs.log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	if err := fs.files.Remove(fh); err != nil {
		fs.log.Error(err)
		return -fuselib.EBADF
	}

	return fusecom.OperationSuccess
}

func (fs *FileSystem) Getattr(path string, stat *fuselib.Stat_t, fh uint64) int {
	fs.log.Debugf("Getattr - {%X}%q", fh, path)

	switch path {
	case "":
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT

	case "/":
		stat.Mode = fuselib.S_IFDIR
		fusecom.ApplyPermissions(false, &stat.Mode)
		return fusecom.OperationSuccess

	default:
		// expectation is to receive `/${multihash}`, not `${namespace}/${mulithash}`
		// we'll do that
		// TODO: [investigate] [b6150f2f-8689-4e60-a605-fd40c826c32d]
		// either `joinRoot` should be exported; ipfscore.GetAttr should take a namespace
		// or something else
		fullPath := corepath.New(gopath.Join("/", strings.ToLower(fs.namespace.String()), path))

		iStat, _, err := ipfscore.GetAttr(fs.Ctx(), fullPath, fs.Core(), transform.IPFSStatRequestAll)
		if err != nil {
			fs.log.Error(err)
			return -fuselib.ENOENT
		}

		*stat = *iStat.ToFuse()
		fusecom.ApplyPermissions(false, &stat.Mode)
		return fusecom.OperationSuccess
	}
}

func (fs *FileSystem) Read(path string, buff []byte, ofst int64, fh uint64) int {
	fs.log.Debugf("Read - {%X}%q", fh, path)

	// TODO: [review] we need to do things on failure
	// the OS typically triggers a close, but we shouldn't expect it to invalidate this record for us
	// we also might want to store a file cursor to reduce calls to seek
	// the same thing already happens internally so it's at worst the overhead of a call right now

	if fh == fusecom.ErrorHandle {
		fs.log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

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

	readBytes, err := file.Read(buff)
	if err != nil && err != io.EOF {
		fs.log.Errorf("Read - error: %s", err)
		return -fuselib.EIO
	}
	return readBytes
}
func (fs *FileSystem) Readdir(path string,
	fill func(name string, stat *fuselib.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) int {
	fs.log.Debugf("Readdir - {%X|%d}%q", fh, ofst, path)

	if fh == fusecom.ErrorHandle {
		fs.log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

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

func (fs *FileSystem) Readlink(path string) (int, string) {
	fs.log.Debugf("Readlink - %q", path)

	switch path {
	default:
		break

	case "/":
		fs.log.Warnf("Readlink - root path is an invalid request")
		return -fuse.EINVAL, ""

	case "":
		fs.log.Error("Readlink - empty request")
		return -fuselib.ENOENT, ""
	}

	// TODO: timeout contexts
	corePath := corepath.New(path[1:])
	iStat, _, err := ipfscore.GetAttr(fs.Ctx(), corePath, fs.Core(), transform.IPFSStatRequest{Type: true})
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

	return len(linkActual.Target), linkActual.Target
}

func (fs *FileSystem) Create(path string, flags int, mode uint32) (int, uint64) {
	fs.log.Debugf("Create - {%X|%X}%q", flags, mode, path)
	return fs.Open(path, flags) // TODO: implement for real
}
