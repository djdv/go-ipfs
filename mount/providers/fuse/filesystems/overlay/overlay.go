package overlay

import (
	"context"
	"errors"
	"strings"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	"github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/ipfs"
	"github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/mfs"
	"github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/pinfs"
	mountcom "github.com/ipfs/go-ipfs/mount/utils/common"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var log = logging.Logger("fuse/overlay")
var _ fuselib.FileSystemInterface = (*FileSystem)(nil)

type FileSystem struct {
	//provcom.IPFSCore
	//provcom.MFS
	initFunc        func() error // we don't need to access initialization data more than once, so we'll compose it in New() and dispose of it after Init()
	ipfs, ipns, mfs fuselib.FileSystemInterface
	initChan        fusecom.InitSignal
}

func NewFileSystem(ctx context.Context, core coreiface.CoreAPI, opts ...Option) *FileSystem {
	options := new(options)
	for _, opt := range opts {
		opt.apply(options)
	}
	if options.resourceLock == nil {
		options.resourceLock = mountcom.NewResourceLocker()
	}

	var (
		overlay    = &FileSystem{initChan: options.initSignal}
		subsystems = 2
		initChan   = make(fusecom.InitSignal)
	)

	mRoot := options.filesAPIRoot
	if mRoot != nil {
		subsystems++
	}

	subInits := make([]func(), 0, subsystems)

	{ // attach /ipfs
		// TODO: use concrete type when it implements the full FS; the compiler hates embedding right now
		//var pinfsSub pinfs.FileSystem
		var pinfsSub fuselib.FileSystemInterface

		ipfsSub := ipfs.NewFileSystem(ctx, core, []ipfs.Option{
			ipfs.WithParent(pinfsSub),
			ipfs.WithInitSignal(initChan),
			ipfs.WithResourceLock(options.resourceLock),
		}...)
		subInits = append(subInits, ipfsSub.Init)

		pinfsSub = pinfs.NewFileSystem(ctx, core, []pinfs.Option{
			pinfs.WithParent(overlay),
			pinfs.WithInitSignal(initChan),
			pinfs.WithResourceLock(options.resourceLock),
			pinfs.WithProxy(ipfsSub),
		}...)
		subInits = append(subInits, pinfsSub.Init)

		overlay.ipfs = pinfsSub
	}

	{ // /ipns
		/*
			ipnsSub := &ipns.Filesystem{Base: provcom.NewBase(ctx)} //TODO: use actual
			subInits = append(subInits, ipnsSub.Init)
			overlay.ipns = ipnsSub
		*/
		// [DBG] just clone IPFS for now
		ipnsSub := ipfs.NewFileSystem(ctx, core, []ipfs.Option{
			ipfs.WithParent(overlay),
			ipfs.WithInitSignal(initChan),
		}...)
		subInits = append(subInits, ipnsSub.Init)
		overlay.ipns = ipnsSub
	}

	{ // /file
		if mRoot != nil {
			mfsSub := new(mfs.FileSystem) //TODO: actual
			subInits = append(subInits, mfsSub.Init)
			overlay.mfs = mfsSub
		}
	}

	overlay.initFunc = func() error {
		for _, init := range subInits {
			go init()
			if err := <-initChan; err != nil {
				return err
			}
		}
		return nil
	}

	return overlay
}

// TODO: we can fetch the calling function from runtime
// we should investigate if we can fetch its argument stack and automatically proxy the request
// ^ don't be crazy though

// string: subpath
func (fs *FileSystem) selectFS(path string) (fuselib.FileSystemInterface, string, error) {
	switch path {
	case "":
		return nil, "", errors.New("empty path")
	case "/":
		return fs, "", nil
	default:
		if path[0] != '/' {
			return nil, "", errors.New("invalid path")
		}

		i := 1 // skip leading slash
		i += strings.IndexRune(path[1:], '/')

		var namespace, pathRemainder string
		if i == 0 { // input looks like: `/namespace`
			namespace = path[1:]
			pathRemainder = "/"
		} else { // input looks like: `/namespace/sub...`
			namespace = path[1:i]
			pathRemainder = path[i:]
		}
		switch namespace {
		case "":
			return fs, pathRemainder, nil
		case "ipfs":
			return fs.ipfs, pathRemainder, nil
		case "ipns":
			return fs.ipns, pathRemainder, nil
		case "file":
			if fs.mfs == nil {
				return nil, "", errors.New("TODO: real error")
			}
			return fs.mfs, pathRemainder, nil
		default:
			return nil, "", errors.New("TODO: real error")
		}

	}
}

func (fs *FileSystem) Init() {
	err := fs.initFunc()
	fs.initFunc = nil

	defer log.Debug("init finished")
	if c := fs.initChan; c != nil {
		if err != nil {
			c <- err
		}
		c <- nil
	}
}

func (fs *FileSystem) Destroy() {
}

func (fs *FileSystem) Statfs(path string, stat *fuselib.Statfs_t) int {
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Mknod(path string, mode uint32, dev uint64) int {
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Mkdir(path string, mode uint32) int {
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Unlink(path string) int {
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Rmdir(path string) int {
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Link(oldpath string, newpath string) int {
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Symlink(target string, newpath string) int {
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Readlink(path string) (int, string) {
	return -fuselib.ENOSYS, ""
}

func (fs *FileSystem) Rename(oldpath string, newpath string) int {
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Chmod(path string, mode uint32) int {
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Chown(path string, uid uint32, gid uint32) int {
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Utimens(path string, tmsp []fuselib.Timespec) int {
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Access(path string, mask uint32) int {
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Create(path string, flags int, mode uint32) (int, uint64) {
	log.Debugf("Create - {%X|%X}%q", flags, mode, path)
	return fs.Open(path, flags) // TODO: implement for real
}

func (fs *FileSystem) Open(path string, flags int) (int, uint64) {
	log.Debugf("Open - {%X}%q", flags, path)
	/* TODO: verify this; source libfuse docs
	Creation (O_CREAT, O_EXCL, O_NOCTTY) flags will be filtered out / handled by the kernel.
	Access modes (O_RDONLY, O_WRONLY, O_RDWR, O_EXEC, O_SEARCH) should be used by the filesystem to check if the operation is permitted. If the -o default_permissions mount option is given, this check is already done by the kernel before calling open() and may thus be omitted by the filesystem.
	*/

	// TODO: verify this
	// go fuselib handles O_DIRECTORY for us, if dir operations are performed here; assume open(..., O_DIRECTORY) was passed

	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		panic(err) // FIXME: TODO: handle appropriately
	}

	if targetFs == fs {
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}

	return targetFs.Open(remainder, flags)
}

func (fs *FileSystem) Getattr(path string, stat *fuselib.Stat_t, fh uint64) int {
	log.Debugf("Getattr - {%X}%q", fh, path)
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		panic(err) // FIXME: TODO: handle appropriately
	}
	// TODO: implement for real
	if targetFs == fs {
		switch remainder {
		case "ipfs", "ipns", "file", "":
			stat.Mode = fuselib.S_IFDIR | 0555
			return fusecom.OperationSuccess
		default:
			return -fuselib.ENOENT
		}
	}
	return targetFs.Getattr(remainder, stat, fh)
}

func (fs *FileSystem) Truncate(path string, size int64, fh uint64) int {
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Read(path string, buff []byte, ofst int64, fh uint64) int {
	log.Debugf("Read - {%X}%q", fh, path)

	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		panic(err) // FIXME: TODO: handle appropriately
	}

	if targetFs == fs {
		return -fuselib.EISDIR
	}

	return targetFs.Read(remainder, buff, ofst, fh)
}

func (fs *FileSystem) Write(path string, buff []byte, ofst int64, fh uint64) int {
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Flush(path string, fh uint64) int {
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Release(path string, fh uint64) int {
	log.Debugf("Release - {%X}%q", fh, path)
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		panic(err) // FIXME: TODO: handle appropriately
	}

	if targetFs == fs {
		return fusecom.OperationSuccess // TODO: implement for real
	}

	return targetFs.Release(remainder, fh)
}

func (fs *FileSystem) Fsync(path string, datasync bool, fh uint64) int {
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Opendir(path string) (int, uint64) {
	log.Debugf("Opendir - %q", path)
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		panic(err) // FIXME: TODO: handle appropriately
	}

	if targetFs == fs {
		return fusecom.OperationSuccess, 0 // TODO: implement for real
	}

	return targetFs.Opendir(remainder)

}

func (fs *FileSystem) Readdir(path string,
	fill func(name string, stat *fuselib.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) int {
	log.Debugf("Readdir - {%X|%d}%q", fh, ofst, path)

	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		panic(err) // FIXME: TODO: handle appropriately
	}

	if targetFs == fs {
		fill(".", nil, 0)
		fill("..", nil, 0)
		fill("ipfs", nil, 0)
		fill("ipns", nil, 0)
		if fs.mfs != nil {
			fill("file", nil, 0)
		}
		return fusecom.OperationSuccess // TODO: implement for real
	}

	return targetFs.Readdir(remainder, fill, ofst, fh)
}

func (fs *FileSystem) Releasedir(path string, fh uint64) int {
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		panic(err) // FIXME: TODO: handle appropriately
	}

	if targetFs == fs {
		return fusecom.OperationSuccess // TODO: implement for real
	}

	return targetFs.Releasedir(remainder, fh)
}

func (fs *FileSystem) Fsyncdir(path string, datasync bool, fh uint64) int {
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Setxattr(path string, name string, value []byte, flags int) int {
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Getxattr(path string, name string) (int, []byte) {
	return -fuselib.ENOSYS, nil
}

func (fs *FileSystem) Removexattr(path string, name string) int {
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Listxattr(path string, fill func(name string) bool) int {
	return -fuselib.ENOSYS
}
