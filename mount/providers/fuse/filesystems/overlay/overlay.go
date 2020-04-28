package overlay

import (
	"context"
	"errors"
	"fmt"
	"strings"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	ipfscore "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/core"
	"github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/pinfs"
	mountcom "github.com/ipfs/go-ipfs/mount/utils/common"
	logging "github.com/ipfs/go-log"
	gomfs "github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var _ fuselib.FileSystemInterface = (*FileSystem)(nil)

type FileSystem struct {
	provcom.IPFSCore
	//provcom.MFS

	// init relevant - do not use outside of init(); they will be nil
	initChan    fusecom.InitSignal
	resLock     mountcom.ResourceLock // call methods on fs.(Request|Release) instead
	filesRoot   *gomfs.Root           // use fs.filesAPI after it's initalized
	directories []string

	// FIXME: zap logger implies newly created logs will respect the zapconfig's set Level
	// however this doesn't seem to be the case in go-log
	// `ipfs daemon --debug` will not print debug log for these created logs despite being spawned from a config who's level should now be set to `debug`

	// persistant
	log                  logging.EventLogger
	ipfs, ipns, filesAPI fuselib.FileSystemInterface
}

func NewFileSystem(ctx context.Context, core coreiface.CoreAPI, opts ...Option) *FileSystem {
	settings := parseOptions(opts...)

	return &FileSystem{
		IPFSCore: provcom.NewIPFSCore(ctx, core, settings.ResourceLock),
		initChan: settings.InitSignal,
		log:      settings.Log,
		resLock:  settings.ResourceLock,
	}
}

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
			if fs.filesAPI == nil {
				return nil, "", errors.New("mfs is not attached")
			}
			return fs.filesAPI, pathRemainder, nil
		default:
			return nil, "", fmt.Errorf("requested subsystem %q is not attached", namespace)
		}

	}
}

func (fs *FileSystem) Init() {
	var retErr error
	defer func() {
		if retErr != nil {
			fs.log.Errorf("init failed: %s", retErr)
		}

		if c := fs.initChan; c != nil {
			c <- retErr
			close(fs.initChan)
		}

		fs.log.Errorf("init finished")
	}()

	ipfsSub, err := fs.attachPinFS()
	if err != nil {
		retErr = err
		return
	}
	fs.ipfs = ipfsSub

	ipnsSub, err := fs.attachIPNS()
	if err != nil {
		retErr = err
		return
	}
	fs.ipns = ipnsSub

	fs.directories = []string{
		".",
		"..",
		"ipfs",
		"ipns",
	}

	if fs.filesRoot != nil {
		filesSub, err := fs.attachFilesAPI()
		if err != nil {
			retErr = err
			return
		}
		fs.filesAPI = filesSub
		capacity := len(fs.directories) + 1 // this slice lives forever; so cap the reslice to save less bytes than this comment takes
		fs.directories = append(fs.directories, "file")[:capacity:capacity]
	}
}

func (fs *FileSystem) attachPinFS() (fuselib.FileSystemInterface, error) {
	initChan := make(fusecom.InitSignal) // closed by subsystem

	pinfsSubsys := pinfs.NewFileSystem(fs.Ctx(), fs.Core(),
		pinfs.WithCommon(
			fusecom.WithInitSignal(initChan),
			fusecom.WithResourceLock(fs.resLock),
		),
	)

	go pinfsSubsys.Init()
	var retErr error
	for err := range initChan {
		// TODO: [general] zap-ify all the logs
		if err != nil {
			fs.log.Errorf("subsystem init failed:%s", err)
			retErr = err // last err returned but all logged
		}
	}

	return pinfsSubsys, retErr
}

func (fs *FileSystem) attachIPNS() (fuselib.FileSystemInterface, error) {
	initChan := make(fusecom.InitSignal) // closed by subsystem

	// TODO: lint

	commonOpts := []fusecom.Option{
		fusecom.WithInitSignal(initChan),
		fusecom.WithResourceLock(fs.resLock),
	}

	var keyfsSubsys fuselib.FileSystemInterface

	// handle `/ipns/*` requests via core
	ipnsSubsys := ipfscore.NewFileSystem(fs.Ctx(), fs.Core(),
		ipfscore.WithNamespace(mountinter.NamespaceIPNS),
		ipfscore.WithCommon(append(commonOpts,
			fusecom.WithParent(keyfsSubsys))...),
	)

	go ipnsSubsys.Init()
	var retErr error
	for err := range initChan {
		// TODO: [general] zap-ify all the logs
		fs.log.Errorf("subsystem init failed:%s", err)
		retErr = err // last err returned
	}
	if retErr != nil {
		return nil, retErr
	}

	// handle `/ipns` requests via keyfs
	/* TODO
	keyfsSubsys = keyfs.NewFileSystem(fs.Ctx(), fs.Core(), ipfscore.WithCommon(
		append(commonOpts,
			fusecom.WithParent(fs),
		ipfscore.WithNamespace(mountinter.NamespaceIPNS),
	)

	go keyfsSubsys.Init()
	if err := <-initChan; err != nil {
		return nil, err
	}
	*/

	return ipnsSubsys, retErr
}

func (fs *FileSystem) attachFilesAPI() (fuselib.FileSystemInterface, error) {
	/* TODO
	initChan := make(fusecom.InitSignal)
	commonOpts := []fusecom.Option{
		fusecom.WithInitSignal(initChan),
		fusecom.WithResourceLock(fs.resLock),
	}

	if fs.filesRoot == nil {
		return nil, errors.New("files root is nil")
	}

	{ // handle `/file` requests via MFS
		mfsSub := new(mfs.FileSystem)
		fs.filesAPI = mfsSub
	}
	*/

	return nil, errors.New("not implemented yet")
}

func (fs *FileSystem) Destroy() {
	//TODO: call on subsystems
	fs.log.Debugf("Destroy - Requested")
}

func (*FileSystem) Statfs(path string, stat *fuselib.Statfs_t) int {
	return (*fusecom.SharedMethods).Statfs(nil, path, stat)
}

func (fs *FileSystem) Getattr(path string, stat *fuselib.Stat_t, fh uint64) int {
	fs.log.Debugf("Getattr - Request {%X}%q", fh, path)
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT
	}

	if targetFs == fs {
		stat.Mode |= fuselib.S_IFDIR
		fusecom.ApplyPermissions(false, &stat.Mode)
		stat.Uid, stat.Gid, _ = fuselib.Getcontext()
		return fusecom.OperationSuccess
	}

	return targetFs.Getattr(remainder, stat, fh)
}

func (fs *FileSystem) Opendir(path string) (int, uint64) {
	fs.log.Debugf("Opendir - Request %q", path)
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}

	if targetFs == fs {
		return fusecom.OperationSuccess, 0 // TODO: implement for real
	}

	return targetFs.Opendir(remainder)
}

func (fs *FileSystem) Releasedir(path string, fh uint64) int {
	fs.log.Debugf("Releasedir - Request {%X}%q", fh, path)
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EBADF
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
	fs.log.Debugf("Readdir - Request {%X|%d}%q", fh, ofst, path)

	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EBADF
	}

	if targetFs == fs {
		dLen := int64(len(fs.directories))
		if ofst > dLen {
			return -fuselib.ENOENT
		}

		for ofst != dLen {
			name := fs.directories[ofst]
			ofst++
			if !fill(name, nil, ofst) {
				break
			}
		}
		return fusecom.OperationSuccess
	}

	return targetFs.Readdir(remainder, fill, ofst, fh)
}

func (fs *FileSystem) Open(path string, flags int) (int, uint64) {
	fs.log.Debugf("Open - Request {%X}%q", flags, path)
	/* TODO: verify this; source libfuse docs
	Creation (O_CREAT, O_EXCL, O_NOCTTY) flags will be filtered out / handled by the kernel.
	Access modes (O_RDONLY, O_WRONLY, O_RDWR, O_EXEC, O_SEARCH) should be used by the filesystem to check if the operation is permitted. If the -o default_permissions mount option is given, this check is already done by the kernel before calling open() and may thus be omitted by the filesystem.
	*/

	// TODO: verify this
	// go fuselib handles O_DIRECTORY for us, if dir operations are performed here; assume open(..., O_DIRECTORY) was passed

	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}

	if targetFs == fs {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}

	return targetFs.Open(remainder, flags)
}

func (fs *FileSystem) Release(path string, fh uint64) int {
	fs.log.Debugf("Release - Request {%X}%q", fh, path)
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EBADF
	}

	if targetFs == fs {
		return fusecom.OperationSuccess // TODO: implement for real
	}

	return targetFs.Release(remainder, fh)
}

func (fs *FileSystem) Read(path string, buff []byte, ofst int64, fh uint64) int {
	fs.log.Debugf("Read - Request {%X|%d}%q", fh, ofst, path)

	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EBADF
	}

	if targetFs == fs {
		fs.log.Error(fuselib.Error(-fuselib.EISDIR))
		return -fuselib.EISDIR
	}

	return targetFs.Read(remainder, buff, ofst, fh)
}

func (fs *FileSystem) Readlink(path string) (int, string) {
	fs.log.Debugf("Readlink - Request %q", path)
	switch path {
	default:
		targetFs, remainder, err := fs.selectFS(path)
		if err != nil {
			fs.log.Error(err)
			return -fuselib.ENOENT, ""
		}

		return targetFs.Readlink(remainder)

	case "/":
		fs.log.Warnf("Readlink - root path is an invalid request")
		return -fuselib.EINVAL, ""

	case "":
		fs.log.Error("Readlink - empty request")
		return -fuselib.ENOENT, ""

	}
}

func (fs *FileSystem) Create(path string, flags int, mode uint32) (int, uint64) {
	fs.log.Debugf("Create - Request {%X|%X}%q", flags, mode, path)

	switch path {
	default:
		targetFs, remainder, err := fs.selectFS(path)
		if err != nil {
			fs.log.Error(err)
			return -fuselib.ENOENT, fusecom.ErrorHandle
		}

		return targetFs.Create(remainder, flags, mode)

	case "":
		fs.log.Error("Create - empty request")
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}
}

// boilerplate
// TODO: consider writing a code generator BaseFileSystem -> proxy implementation with selector template

func (fs *FileSystem) Access(path string, mask uint32) int {
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOSYS
	}

	return targetFs.Access(remainder, mask)
}

func (fs *FileSystem) Setxattr(path string, name string, value []byte, flags int) int {
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}

	return targetFs.Setxattr(remainder, name, value, flags)
}

func (fs *FileSystem) Getxattr(path string, name string) (int, []byte) {
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT, nil
	}

	return targetFs.Getxattr(remainder, name)
}

func (fs *FileSystem) Removexattr(path string, name string) int {
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}

	return targetFs.Removexattr(remainder, name)
}

func (fs *FileSystem) Listxattr(path string, fill func(name string) bool) int {
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}

	return targetFs.Listxattr(remainder, fill)
}

func (fs *FileSystem) Chmod(path string, mode uint32) int {
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}

	return targetFs.Chmod(remainder, mode)
}

func (fs *FileSystem) Chown(path string, uid uint32, gid uint32) int {
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}

	return targetFs.Chown(remainder, uid, gid)
}

func (fs *FileSystem) Utimens(path string, tmsp []fuselib.Timespec) int {
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}

	return targetFs.Utimens(remainder, tmsp)
}

func (fs *FileSystem) Mknod(path string, mode uint32, dev uint64) int {
	targetFs, remainder, err := fs.selectFS(path)
	fs.log.Errorf("mknod test fs: %#v path:%q, remainder: %q, err: %s", targetFs, path, remainder, err)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}

	return targetFs.Mknod(remainder, mode, dev)
}

func (fs *FileSystem) Truncate(path string, size int64, fh uint64) int {
	targetFs, remainder, err := fs.selectFS(path)
	fs.log.Errorf("truncate test fs: %#v path:%q, remainder: %q, err: %s", targetFs, path, remainder, err)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}

	return targetFs.Truncate(remainder, size, fh)
}

func (fs *FileSystem) Write(path string, buff []byte, ofst int64, fh uint64) int {
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.EBADF
	}

	return targetFs.Write(remainder, buff, ofst, fh)
}

func (fs *FileSystem) Link(oldpath string, newpath string) int {
	targetFs, remainder, err := fs.selectFS(oldpath)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}

	return targetFs.Link(remainder, newpath)
}

func (fs *FileSystem) Unlink(path string) int {
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}

	return targetFs.Unlink(remainder)
}

func (fs *FileSystem) Mkdir(path string, mode uint32) int {
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}

	return targetFs.Mkdir(remainder, mode)
}

func (fs *FileSystem) Rmdir(path string) int {
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}

	return targetFs.Rmdir(remainder)
}

func (fs *FileSystem) Symlink(target string, newpath string) int {
	targetFs, remainder, err := fs.selectFS(newpath)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}

	return targetFs.Symlink(target, remainder)
}

// TODO: review
func (fs *FileSystem) Rename(oldpath string, newpath string) int {
	return -fuselib.ENOSYS

	targetFs, oldRemainder, err := fs.selectFS(oldpath)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}

	_, newRemainder, err := fs.selectFS(newpath)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}

	return targetFs.Symlink(oldRemainder, newRemainder)
}

func (fs *FileSystem) Flush(path string, fh uint64) int {
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.EBADF
	}
	return targetFs.Flush(remainder, fh)
}

func (fs *FileSystem) Fsync(path string, datasync bool, fh uint64) int {
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.EBADF
	}
	return targetFs.Fsync(remainder, datasync, fh)
}

func (fs *FileSystem) Fsyncdir(path string, datasync bool, fh uint64) int {
	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.EBADF
	}
	return targetFs.Fsyncdir(remainder, datasync, fh)
}
