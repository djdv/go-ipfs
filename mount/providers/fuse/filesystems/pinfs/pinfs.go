package pinfs

import (
	"context"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	mountcom "github.com/ipfs/go-ipfs/mount/utils/common"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

const todoHandle = 0

var log = logging.Logger("fuse/pinfs")

var _ fuselib.FileSystemInterface = (*FileSystem)(nil)

type FileSystem struct {
	provcom.IPFSCore
	initChan fusecom.InitSignal

	pinDir transform.Directory
	proxy  fuselib.FileSystemInterface

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

	return &FileSystem{
		IPFSCore: provcom.NewIPFSCore(ctx, core, options.resourceLock),
		initChan: options.initSignal,
		proxy:    options.proxy,
	}
}

func (fs *FileSystem) Init() {
	fs.Lock()
	defer fs.Unlock()
	log.Debug("init")

	/*
		fs.handles = make(fsHandles)
		fs.mountTime = fuselib.Now()
	*/

	defer log.Debug("init finished")
	if c := fs.initChan; c != nil {
		c <- nil
	}
}

func (fs *FileSystem) Open(path string, flags int) (int, uint64) {
	log.Debugf("Open - {%X}%q", flags, path)

	switch path { // TODO: use fh instead of path for this
	case "":
		// TODO: handle empty path (valid if fh is valid)
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
		// TODO: async shenanigans
		fs.pinDir = transform.OpenDirPinfs(fs.Ctx(), fs.Core())
		return fusecom.OperationSuccess, todoHandle

	default:
		if fs.proxy != nil {
			return fs.proxy.Opendir(path)
		}
		log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}
}

func (fs *FileSystem) Releasedir(path string, fh uint64) int {
	switch path {
	case "": // TODO: valid if fh is valid
		log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF

	case "/":
		// TODO: async shenanigans
		if fs.pinDir == nil {
			log.Error(fuselib.Error(-fuselib.EBADF))
			return -fuselib.EBADF
		}
		return fusecom.OperationSuccess

	default:
		if fs.proxy != nil {
			return fs.proxy.Releasedir(path, fh)
		}
		log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}
}

func (fs *FileSystem) Release(path string, fh uint64) int {
	switch path {
	case "": // TODO: valid if fh is valid
		log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF

	case "/":
		log.Errorf("wrong method Release, expecting Releasedir")
		log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF

	default:
		if fs.proxy != nil {
			return fs.proxy.Release(path, fh)
		}
		log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}
}

func (fs *FileSystem) Getattr(path string, stat *fuselib.Stat_t, fh uint64) (errc int) {
	log.Debugf("Getattr - {%X}%q", fh, path)

	switch path { // TODO: use fh instead of path for this
	// TODO: handle empty path (valid if fh is valid)
	case "/":
		stat.Mode = fuselib.S_IFDIR | 0555
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
	switch path { // TODO: use fh instead of path for this
	case "/":
		log.Error(fuselib.Error(-fuselib.EISDIR))
		return -fuselib.EISDIR
	default:
		if fs.proxy != nil {
			return fs.proxy.Read(path, buff, ofst, fh)
		}
		log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}
}

func (fs *FileSystem) Readdir(path string,
	fill func(name string, stat *fuselib.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) int {
	log.Debugf("Readdir - {%X|%d}%q", fh, ofst, path)

	// TODO: use fh instead of path for this
	if path != "/" && fs.proxy != nil {
		return fs.proxy.Readdir(path, fill, ofst, fh)
	}

	if fs.pinDir == nil {
		log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	// TODO: [audit] int -> uint needs range checking
	entChan, err := fs.pinDir.Read(uint64(ofst), 0).ToFuse()
	if err != nil {
		// TODO: inspect error
		log.Error(err)
		return -fuselib.EBADF
	}

	// TODO: populate these stats
	fill(".", nil, 0)
	fill("..", nil, 0) // if parent !nil; parent.Getattr().ToFuse(

	for ent := range entChan {
		fusecom.ApplyPermissions(false, &ent.Stat.Mode)
		if !fill(ent.Name, ent.Stat, ent.Offset) {
			log.Debugf("Readdir - aborting, fill dropped {[%X]%q}", fh, path)
			break
		}
	}

	return fusecom.OperationSuccess
}
