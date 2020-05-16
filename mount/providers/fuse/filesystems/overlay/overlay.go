package overlay

import (
	"context"
	"errors"
	"fmt"
	"strings"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	"github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/keyfs"
	"github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/mfs"
	"github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/pinfs"
	mountcom "github.com/ipfs/go-ipfs/mount/utils/common"
	logging "github.com/ipfs/go-log"
	gomfs "github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var _ fuselib.FileSystemInterface = (*FileSystem)(nil)

const rootHandle = 42 // we handle directory listings on the fly, no need for dynamic directory objects/handles

type FileSystem struct {
	provcom.IPFSCore
	//provcom.MFS

	// init relevant
	initChan     fusecom.InitSignal
	resLock      mountcom.ResourceLock // don't reference directly, call methods on fs.(Request|Release)
	filesAPIRoot *gomfs.Root           // don't reference directly, use fs.filesAPI after it's initialized
	directories  []string

	// FIXME: zap logger implies newly created logs will respect the zapconfig's set Level
	// however this doesn't seem to be the case in go-log
	// `ipfs daemon --debug` will not print debug log for these created logs despite being spawned from a config who's level should now be set to `debug`

	// persistent
	log                  logging.EventLogger
	ipfs, ipns, filesAPI fuselib.FileSystemInterface

	mountTimeGroup fusecom.StatTimeGroup
	rootStat       *fuselib.Stat_t
}

func NewFileSystem(ctx context.Context, core coreiface.CoreAPI, opts ...Option) *FileSystem {
	settings := parseOptions(opts...)

	return &FileSystem{
		IPFSCore:     provcom.NewIPFSCore(ctx, core, settings.ResourceLock),
		initChan:     settings.InitSignal,
		log:          settings.Log,
		resLock:      settings.ResourceLock,
		filesAPIRoot: settings.filesAPIRoot,
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

		slashIndex := 1 // skip leading slash
		slashIndex += strings.IndexRune(path[1:], '/')

		var namespace, pathRemainder string
		if slashIndex == 0 { // input looks like: `/namespace`
			namespace = path[1:]
			pathRemainder = "/"
		} else { // input looks like: `/namespace/sub...`
			namespace = path[1:slashIndex]
			pathRemainder = path[slashIndex:]
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
			if retErr != nil {
				c <- retErr
			}
			close(c)
		}

		fs.log.Debugf("init finished")
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

	if fs.filesAPIRoot != nil {
		filesSub, err := fs.attachFilesAPI()
		if err != nil {
			retErr = err
			return
		}
		fs.filesAPI = filesSub
		capacity := len(fs.directories) + 1 // this slice lives forever; so cap the reslice to save less bytes than this comment takes
		fs.directories = append(fs.directories, "file")[:capacity:capacity]
	}

	timeOfMount := fuselib.Now()

	fs.mountTimeGroup = fusecom.StatTimeGroup{
		Atim:     timeOfMount,
		Mtim:     timeOfMount,
		Ctim:     timeOfMount,
		Birthtim: timeOfMount,
	}

	fs.rootStat = &fuselib.Stat_t{
		Mode: fuselib.S_IFDIR | fusecom.IRXA&^(fuselib.S_IXOTH), // |0554
		Atim: timeOfMount,
		Mtim: timeOfMount,
		Ctim: timeOfMount,
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

	// handle `/ipns` requests via keyfs
	keyfsSubsys := keyfs.NewFileSystem(fs.Ctx(), fs.Core(),
		keyfs.WithCommon(
			fusecom.WithInitSignal(initChan),
			fusecom.WithResourceLock(fs.IPFSCore),
			fusecom.WithParent(fs),
		),
	)
	go keyfsSubsys.Init()

	var retErr error
	for err := range initChan {
		// TODO: [general] zap-ify all the logs
		fs.log.Errorf("subsystem init failed:%s", err)
		retErr = err // last err returned
	}
	if retErr != nil {
		return nil, retErr
	}

	return keyfsSubsys, retErr
}

func (fs *FileSystem) attachFilesAPI() (fuselib.FileSystemInterface, error) {
	initChan := make(fusecom.InitSignal)

	if fs.filesAPIRoot == nil {
		return nil, errors.New("files root is nil")
	}

	// handle `/file` requests via MFS
	fileSubsys := mfs.NewFileSystem(fs.Ctx(), *fs.filesAPIRoot, fs.Core(),
		mfs.WithCommon(
			fusecom.WithInitSignal(initChan),
			fusecom.WithResourceLock(fs.IPFSCore),
			fusecom.WithParent(fs),
		),
	)
	go fileSubsys.Init()

	var retErr error
	for err := range initChan {
		// TODO: [general] zap-ify all the logs
		fs.log.Errorf("subsystem init failed:%s", err)
		retErr = err // last err returned
	}
	if retErr != nil {
		return nil, retErr
	}

	return fileSubsys, nil
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
		*stat = *fs.rootStat
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
		return fusecom.OperationSuccess, rootHandle
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
		if fh != rootHandle {
			return -fuselib.EBADF
		}

		return fusecom.OperationSuccess
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

	goErr, errNo := fusecom.CheckOpenFlagsBasic(false, flags)
	if goErr != nil {
		fs.log.Error(goErr)
		return errNo, fusecom.ErrorHandle
	}

	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}

	if targetFs == fs {
		fs.log.Errorf("tried to open the root as a file")
		return -fuselib.EISDIR, fusecom.ErrorHandle
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
		fs.log.Errorf("tried to release the root as a file")
		return -fuselib.EBADF
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

// TODO: needs test
func (fs *FileSystem) Rename(oldpath string, newpath string) int {
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
