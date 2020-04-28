package pinfs

import (
	"context"

	"github.com/billziss-gh/cgofuse/fuse"
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	ipfscore "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/core"
	mountcom "github.com/ipfs/go-ipfs/mount/utils/common"
	pinfs "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/pinfs"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var _ fuselib.FileSystemInterface = (*FileSystem)(nil)

type FileSystem struct {
	fusecom.SharedMethods
	provcom.IPFSCore

	initChan fusecom.InitSignal

	directories fusecom.DirectoryTable

	log     logging.EventLogger
	ipfs    fuselib.FileSystemInterface
	resLock mountcom.ResourceLock
}

func NewFileSystem(ctx context.Context, core coreiface.CoreAPI, opts ...Option) *FileSystem {
	settings := parseOptions(opts...)

	return &FileSystem{
		IPFSCore: provcom.NewIPFSCore(ctx, core, settings.ResourceLock),
		initChan: settings.InitSignal,
		log:      settings.Log,
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

	// proxy subrequests to IPFS
	initChan := make(fusecom.InitSignal)
	ipfsSubsys := ipfscore.NewFileSystem(fs.Ctx(), fs.Core(),
		ipfscore.WithNamespace(mountinter.NamespaceIPFS),
		ipfscore.WithCommon(
			fusecom.WithInitSignal(initChan),
			fusecom.WithParent(fs),
			fusecom.WithResourceLock(fs.resLock),
		),
	)

	go ipfsSubsys.Init()
	for err := range initChan {
		if err != nil {
			fs.log.Errorf("subsystem init failed:%s", err)
			retErr = err // last err returned but all logged
		}
	}

	fs.ipfs = ipfsSubsys

	// fs.mountTime = fuselib.Now()
	fs.directories = fusecom.NewDirectoryTable()
}

func (fs *FileSystem) Destroy() {
	fs.log.Debugf("Destroy - Requested")
}

func (fs *FileSystem) Getattr(path string, stat *fuselib.Stat_t, fh uint64) (errc int) {
	fs.log.Debugf("Getattr - {%X}%q", fh, path)

	switch path {
	case "":
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT

	case "/":
		// TODO: consider adding write permissions and allowing `rmdir()`
		// mapping it to unpin
		// this isn't POSIX compliant so tools won't work with it by default
		// but would be useful if documented
		stat.Mode = fuselib.S_IFDIR
		fusecom.ApplyPermissions(false, &stat.Mode)
		stat.Uid, stat.Gid, _ = fuselib.Getcontext()
		return fusecom.OperationSuccess

	default:
		return fs.ipfs.Getattr(path, stat, fh)
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}
}

// TODO: not finished; doesn't care about path yet
func (fs *FileSystem) Opendir(path string) (int, uint64) {
	fs.log.Debugf("Opendir - %q", path)

	switch path {
	case "":
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT, fusecom.ErrorHandle

	case "/":
		pinDir, err := pinfs.OpenDir(fs.Ctx(), fs.Core())
		if err != nil { // TODO: inspect/transform error
			fs.log.Error(err)
			return -fuselib.ENOENT, fusecom.ErrorHandle
		}
		handle, err := fs.directories.Add(pinDir)
		if err != nil { // TODO: inspect/transform error
			fs.log.Error(err)
			return -fuselib.ENFILE, fusecom.ErrorHandle
		}
		return fusecom.OperationSuccess, handle

	default:
		return fs.ipfs.Opendir(path)
	}
}

func (fs *FileSystem) Releasedir(path string, fh uint64) int {
	fs.log.Debugf("Releasedir - {%X}%q", fh, path)

	if path == "/" {
		if err := fs.directories.Remove(fh); err != nil {
			fs.log.Error(err)
			return -fuselib.EBADF
		}
		return fusecom.OperationSuccess
	}

	return fs.ipfs.Releasedir(path, fh)
}

func (fs *FileSystem) Readdir(path string,
	fill func(name string, stat *fuselib.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) int {
	fs.log.Debugf("Readdir - {%X|%d}%q", fh, ofst, path)

	if path != "/" {
		return fs.ipfs.Readdir(path, fill, ofst, fh)
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

func (fs *FileSystem) Open(path string, flags int) (int, uint64) {
	fs.log.Debugf("Open - {%X}%q", flags, path)

	switch path {
	case "":
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT, fusecom.ErrorHandle

	case "/":
		fs.log.Error(fuselib.Error(-fuselib.EISDIR))
		return -fuselib.EISDIR, fusecom.ErrorHandle

	default:
		return fs.ipfs.Open(path, flags)
	}
}

func (fs *FileSystem) Release(path string, fh uint64) int {
	fs.log.Debugf("Release - {%X}%q", fh, path)
	return fs.ipfs.Release(path, fh)
}

func (fs *FileSystem) Read(path string, buff []byte, ofst int64, fh uint64) int {
	fs.log.Debugf("Read - {%X}%q", fh, path)
	return fs.ipfs.Read(path, buff, ofst, fh)
}

func (fs *FileSystem) Readlink(path string) (int, string) {
	fs.log.Debugf("Readlink - %q", path)

	switch path {
	default:
		return fs.ipfs.Readlink(path)

	case "/":
		fs.log.Warnf("Readlink - root path is an invalid request")
		return -fuse.EINVAL, ""

	case "":
		fs.log.Error("Readlink - empty request")
		return -fuselib.ENOENT, ""
	}
}
