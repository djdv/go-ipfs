package pinfs

import (
	"context"

	"github.com/billziss-gh/cgofuse/fuse"
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	pinfs "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/pinfs"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var log = logging.Logger("fuse/pinfs")

var _ fuselib.FileSystemInterface = (*FileSystem)(nil)

type FileSystem struct {
	provcom.IPFSCore
	initChan fusecom.InitSignal

	directories fusecom.DirectoryTable
	proxy       fuselib.FileSystemInterface

	// TODO: remove this; packages should implement all interface methods within the package
	fuselib.FileSystemBase
}

func NewFileSystem(ctx context.Context, core coreiface.CoreAPI, opts ...Option) *FileSystem {
	settings := parseOptions(opts...)

	return &FileSystem{
		IPFSCore: provcom.NewIPFSCore(ctx, core, settings.ResourceLock),
		initChan: settings.InitSignal,
		proxy:    settings.proxy,
	}
}

func (fs *FileSystem) Init() {
	fs.Lock()
	defer fs.Unlock()
	log.Debug("init")

	// fs.mountTime = fuselib.Now()
	fs.directories = fusecom.NewDirectoryTable()

	defer log.Debug("init finished")
	if c := fs.initChan; c != nil {
		c <- nil
	}
}

func (fs *FileSystem) Open(path string, flags int) (int, uint64) {
	log.Debugf("Open - {%X}%q", flags, path)

	switch path {
	case "":
		log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT, fusecom.ErrorHandle

	case "/":
		log.Error(fuselib.Error(-fuselib.EISDIR))
		return -fuselib.EISDIR, fusecom.ErrorHandle

	default:
		if fs.proxy != nil {
			return fs.proxy.Open(path, flags)
		}
		log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}
}

// TODO: not finished; doesn't care about path yet
func (fs *FileSystem) Opendir(path string) (int, uint64) {
	log.Debugf("Opendir - %q", path)

	switch path {
	case "":
		log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT, fusecom.ErrorHandle

	case "/":
		pinDir, err := pinfs.OpenDir(fs.Ctx(), fs.Core())
		if err != nil { // TODO: inspect/transform error
			log.Error(err)
			return -fuselib.ENOENT, fusecom.ErrorHandle
		}
		handle, err := fs.directories.Add(pinDir)
		if err != nil { // TODO: inspect/transform error
			log.Error(err)
			return -fuselib.ENFILE, fusecom.ErrorHandle
		}
		return fusecom.OperationSuccess, handle

	default:
		if fs.proxy != nil {
			return fs.proxy.Opendir(path)
		}
		log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}
}

func (fs *FileSystem) Releasedir(path string, fh uint64) int {
	log.Debugf("Releasedir - {%X}%q", fh, path)

	if fh == fusecom.ErrorHandle || fs.proxy == nil {
		log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	if path == "/" {
		if err := fs.directories.Remove(fh); err != nil {
			log.Error(err)
			return -fuselib.EBADF
		}
		return fusecom.OperationSuccess
	}

	return fs.proxy.Releasedir(path, fh)
}

func (fs *FileSystem) Release(path string, fh uint64) int {
	log.Debugf("Release - {%X}%q", fh, path)

	if fh == fusecom.ErrorHandle || fs.proxy == nil {
		log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	return fs.proxy.Release(path, fh)
}

func (fs *FileSystem) Getattr(path string, stat *fuselib.Stat_t, fh uint64) (errc int) {
	log.Debugf("Getattr - {%X}%q", fh, path)

	switch path {
	case "":
		log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT

	case "/":
		// TODO: consider adding write permissions and allowing `rmdir()`
		// mapping it to unpin
		// this isn't POSIX compliant so tools won't work with it by default
		// but would be useful if documented
		stat.Mode = fuselib.S_IFDIR
		fusecom.ApplyPermissions(false, &stat.Mode)
		return fusecom.OperationSuccess

	default:
		if fs.proxy != nil {
			return fs.proxy.Getattr(path, stat, fh)
		}
		log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}
}

func (fs *FileSystem) Read(path string, buff []byte, ofst int64, fh uint64) int {
	log.Debugf("Read - {%X}%q", fh, path)

	if fh == fusecom.ErrorHandle || fs.proxy == nil {
		log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	return fs.proxy.Read(path, buff, ofst, fh)
}

func (fs *FileSystem) Readdir(path string,
	fill func(name string, stat *fuselib.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) int {
	log.Debugf("Readdir - {%X|%d}%q", fh, ofst, path)

	if path != "/" && fs.proxy != nil {
		return fs.proxy.Readdir(path, fill, ofst, fh)
	}

	if fh == fusecom.ErrorHandle {
		log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	directory, err := fs.directories.Get(fh)
	if err != nil {
		log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	goErr, errNo := fusecom.FillDir(directory, false, fill, ofst)
	if goErr != nil {
		log.Error(goErr)
	}

	return errNo
}

func (fs *FileSystem) Readlink(path string) (int, string) {
	log.Debugf("Readlink - %q", path)

	switch path {
	default:
		if fs.proxy != nil {
			return fs.proxy.Readlink(path)
		}
		log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT, ""

	case "/":
		log.Warnf("Readlink - root path is an invalid request")
		return -fuse.EINVAL, ""

	case "":
		log.Error("Readlink - empty request")
		return -fuselib.ENOENT, ""
	}
}

func (fs *FileSystem) Create(path string, flags int, mode uint32) (int, uint64) {
	log.Debugf("Create - {%X|%X}%q", flags, mode, path)
	return fs.Open(path, flags) // TODO: implement for real
}
