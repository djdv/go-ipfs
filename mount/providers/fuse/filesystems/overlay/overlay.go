package overlay

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/billziss-gh/cgofuse/fuse"
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	config "github.com/ipfs/go-ipfs-config"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	ipfscore "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/core"
	"github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/mfs"
	"github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/pinfs"
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
	settings := parseOptions(opts...)

	var (
		overlay    = &FileSystem{initChan: settings.InitSignal}
		subsystems = 2
		initChan   = make(fusecom.InitSignal)
		commonOpts = []fusecom.Option{
			fusecom.WithInitSignal(initChan),
			fusecom.WithResourceLock(settings.ResourceLock),
		}
	)

	mRoot := settings.filesAPIRoot
	if mRoot != nil {
		subsystems++
	}

	subInits := make([]func(), 0, subsystems)

	{ // attach /ipfs + /ipns

		// pinfs + ipfs
		var (
			pinfsSub *pinfs.FileSystem
			// keyfsSub *keyfs.FileSystem // TODO
		)

		ipfsSub := ipfscore.NewFileSystem(ctx, core, ipfscore.WithCommon(
			append(commonOpts,
				fusecom.WithParent(pinfsSub),
				fusecom.WithLog(logging.Logger("fuse/ipfs")))...),
			ipfscore.WithNamespace(mountinter.NamespaceIPFS),
		)
		subInits = append(subInits, ipfsSub.Init)

		pinfsSub = pinfs.NewFileSystem(ctx, core, pinfs.WithCommon(
			append(commonOpts,
				fusecom.WithParent(overlay))...),
			pinfs.WithProxy(ipfsSub),
		)

		subInits = append(subInits, pinfsSub.Init)

		overlay.ipfs = pinfsSub

		// keyfs + ipns
		// TODO: populate keyfs
		//ipnsSub := ipns.NewFileSystem(ctx, core, append(coreOpts, ipfscore.WithParent(keyfsSub))...)
		ipnsSub := ipfscore.NewFileSystem(ctx, core, ipfscore.WithCommon(
			append(commonOpts,
				fusecom.WithParent(overlay),
				fusecom.WithLog(logging.Logger("fuse/ipns")))...),
			ipfscore.WithNamespace(mountinter.NamespaceIPNS),
		)

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
				return nil, "", errors.New("mfs is not attached")
			}
			return fs.mfs, pathRemainder, nil
		default:
			return nil, "", fmt.Errorf("requested subsystem %q is not attached", namespace)
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
	log.Debugf("Destroy - Requested")
}

func (fs *FileSystem) Statfs(path string, stat *fuselib.Statfs_t) int {
	log.Debugf("Statfs - Request %q", path)

	if path == "" || path == "/" {
		target, err := config.DataStorePath("")
		if err != nil {
			log.Errorf("Statfs - Config err %q: %v", path, err)
			return -fuse.ENOENT
		}

		goErr, errNo := fusecom.Statfs(target, stat)
		if err != nil {
			log.Errorf("Statfs - err %q: %v", target, goErr)
		}
		return errNo
	}

	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}

	return targetFs.Statfs(remainder, stat)
}

func (fs *FileSystem) Getattr(path string, stat *fuselib.Stat_t, fh uint64) int {
	log.Debugf("Getattr - Request {%X}%q", fh, path)
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		log.Error(err)
		return -fuselib.ENOENT
	}

	if targetFs == fs {
		stat.Mode |= fuselib.S_IFDIR
		fusecom.ApplyPermissions(false, &stat.Mode)
		return fusecom.OperationSuccess
	}

	return targetFs.Getattr(remainder, stat, fh)
}

func (fs *FileSystem) Opendir(path string) (int, uint64) {
	log.Debugf("Opendir - Request %q", path)
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		panic(err) // FIXME: TODO: handle appropriately
	}

	if targetFs == fs {
		return fusecom.OperationSuccess, 0 // TODO: implement for real
	}

	return targetFs.Opendir(remainder)
}

func (fs *FileSystem) Releasedir(path string, fh uint64) int {
	log.Debugf("Releasedir - Request {%X}%q", fh, path)
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		panic(err) // FIXME: TODO: handle appropriately
	}

	if targetFs == fs {
		return fusecom.OperationSuccess // TODO: implement for real
	}

	return targetFs.Releasedir(remainder, fh)
}

func (fs *FileSystem) Readdir(path string,
	fill func(name string, stat *fuselib.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) int {
	log.Debugf("Readdir - Request {%X|%d}%q", fh, ofst, path)

	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		panic(err) // FIXME: TODO: handle appropriately
	}

	if targetFs == fs {
		fill(".", nil, 0)
		fill("..", nil, 0)
		if fs.ipfs != nil {
			fill("ipfs", nil, 0)
		}
		if fs.ipns != nil {
			fill("ipns", nil, 0)
		}
		if fs.mfs != nil {
			fill("file", nil, 0)
		}
		return fusecom.OperationSuccess // TODO: implement for real
	}

	return targetFs.Readdir(remainder, fill, ofst, fh)
}

func (fs *FileSystem) Open(path string, flags int) (int, uint64) {
	log.Debugf("Open - Request {%X}%q", flags, path)
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
		log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}

	return targetFs.Open(remainder, flags)
}

func (fs *FileSystem) Release(path string, fh uint64) int {
	log.Debugf("Release - Request {%X}%q", fh, path)
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		panic(err) // FIXME: TODO: handle appropriately
	}

	if targetFs == fs {
		return fusecom.OperationSuccess // TODO: implement for real
	}

	return targetFs.Release(remainder, fh)
}

func (fs *FileSystem) Read(path string, buff []byte, ofst int64, fh uint64) int {
	log.Debugf("Read - Request {%X|%d}%q", fh, ofst, path)

	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		panic(err) // FIXME: TODO: handle appropriately
	}

	if targetFs == fs {
		log.Error(fuselib.Error(-fuselib.EISDIR))
		return -fuselib.EISDIR
	}

	return targetFs.Read(remainder, buff, ofst, fh)
}

func (fs *FileSystem) Readlink(path string) (int, string) {
	log.Debugf("Readlink - Request %q", path)
	switch path {
	default:
		targetFs, remainder, err := fs.selectFS(path)
		if err != nil {
			log.Error(fuselib.Error(-fuselib.ENOENT))
			return -fuselib.ENOENT, ""
		}

		return targetFs.Readlink(remainder)

	case "/":
		log.Warnf("Readlink - root path is an invalid request")
		return -fuse.EINVAL, ""

	case "":
		log.Error("Readlink - empty request")
		return -fuse.ENOENT, ""

	}
}

func (fs *FileSystem) Mknod(path string, mode uint32, dev uint64) int {
	log.Errorf("Mknod - Request {%X|%d}%q", mode, dev, path)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Mkdir(path string, mode uint32) int {
	log.Errorf("Mkdir - Request {%X}%q", mode, path)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Unlink(path string) int {
	log.Errorf("Unlink - Request %q", path)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Rmdir(path string) int {
	log.Errorf("Rmdir - Request %q", path)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Link(oldpath string, newpath string) int {
	log.Errorf("Link - Request %q<->%q", oldpath, newpath)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Symlink(target string, newpath string) int {
	log.Errorf("Symlink - Request %q->%q", newpath, target)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Rename(oldpath string, newpath string) int {
	log.Errorf("Rename - Request %q->%q", oldpath, newpath)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Chmod(path string, mode uint32) int {
	log.Errorf("Chmod - Request {%X}%q", mode, path)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Chown(path string, uid uint32, gid uint32) int {
	log.Errorf("Chown - Request {%d|%d}%q", uid, gid, path)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Utimens(path string, tmsp []fuselib.Timespec) int {
	log.Errorf("Utimens - Request {%v}%q", tmsp, path)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Access(path string, mask uint32) int {
	log.Errorf("Access - Request {%X}%q", mask, path)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Create(path string, flags int, mode uint32) (int, uint64) {
	log.Errorf("Create - Request {%X|%X}%q", flags, mode, path)
	return fs.Open(path, flags) // TODO: implement for real
}

func (fs *FileSystem) Truncate(path string, size int64, fh uint64) int {
	log.Errorf("Truncate - Request {%X|%d}%q", fh, size, path)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Write(path string, buff []byte, ofst int64, fh uint64) int {
	log.Errorf("Write - Request {%X|%d|%d}%q", fh, len(buff), ofst, path)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Flush(path string, fh uint64) int {
	log.Debugf("Flush - Request {%X}%q", fh, path)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Fsync(path string, datasync bool, fh uint64) int {
	log.Errorf("Fsync - Request {%X|%t}%q", fh, datasync, path)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Fsyncdir(path string, datasync bool, fh uint64) int {
	log.Errorf("Fsyncdir - Request {%X|%t}%q", fh, datasync, path)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Setxattr(path string, name string, value []byte, flags int) int {
	log.Errorf("Setxattr - Request {%X|%s|%d}%q", flags, name, len(value), path)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Getxattr(path string, name string) (int, []byte) {
	log.Errorf("Getxattr - Request {%s}%q", name, path)
	return -fuselib.ENOSYS, nil
}

func (fs *FileSystem) Removexattr(path string, name string) int {
	log.Errorf("Removexattr - Request {%s}%q", name, path)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Listxattr(path string, fill func(name string) bool) int {
	log.Errorf("Listxattr - Request %q", path)
	return -fuselib.ENOSYS
}
