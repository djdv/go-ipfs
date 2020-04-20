package pinfs

import (
	"context"

	"github.com/billziss-gh/cgofuse/fuse"
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
	case "/":
		return -fuse.EISDIR, fusecom.ErrorHandle
	default:
		if fs.proxy != nil {
			return fs.proxy.Open(path, flags)
		}
		return -fuse.ENOENT, fusecom.ErrorHandle
	}
}

// TODO: not finished; doesn't care about path yet
func (fs *FileSystem) Opendir(path string) (int, uint64) {
	log.Debugf("Opendir - %q", path)

	dir, err := transform.OpenDirPinfs(fs.Ctx(), fs.Core())
	if err != nil {
		// TODO: real values based on err
		return -1, fusecom.ErrorHandle
	}

	fs.pinDir = dir
	return fusecom.OperationSuccess, todoHandle
}

// TODO: not finished; doesn't care about path yet
func (fs *FileSystem) Releasedir(path string, fh uint64) int {
	if fs.pinDir == nil {
		return -fuselib.EBADF
	}

	fs.pinDir = nil
	return fusecom.OperationSuccess
}

func (fs *FileSystem) Release(path string, fh uint64) int {
	return fusecom.OperationSuccess
}

func (fs *FileSystem) Getattr(path string, stat *fuselib.Stat_t, fh uint64) (errc int) {
	log.Debugf("Getattr - {%X}%q", fh, path)

	switch path { // TODO: use fh instead of path for this
	case "/":
		stat.Mode = fuselib.S_IFDIR | 0555
		return fusecom.OperationSuccess
	default:
		if fs.proxy != nil {
			return fs.proxy.Getattr(path, stat, fh)
		}
		return -fuselib.ENOENT
	}
}

func (fs *FileSystem) Read(path string, buff []byte, ofst int64, fh uint64) int {
	log.Debugf("Read - {%X}%q", fh, path)
	switch path { // TODO: use fh instead of path for this
	case "/":
		return -fuse.EISDIR
	default:
		if fs.proxy != nil {
			return fs.proxy.Read(path, buff, ofst, fh)
		}
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
		return -fuse.EBADF
	}

	// TODO: populate these stats
	fill(".", nil, 0)
	fill("..", nil, 0) // if parent !nil; parent.Getattr().ToFuse()

	// TODO: [audit] int -> uint needs range checking
	entChan, err := fs.pinDir.Read(uint64(ofst), 0).ToFuse()
	if err != nil {
		// TODO: inspect error
		log.Error(err)
		return -fuse.EBADF
	}

	for ent := range entChan {
		fusecom.ApplyPermissions(false, &ent.Stat.Mode)
		if !fill(ent.Name, ent.Stat, ent.Offset) {
			log.Debugf("Readdir - aborting, fill dropped {[%X]%q}", fh, path)
			break
		}
	}

	return fusecom.OperationSuccess
}
