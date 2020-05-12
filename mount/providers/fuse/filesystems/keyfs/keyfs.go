package keyfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/billziss-gh/cgofuse/fuse"
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	files "github.com/ipfs/go-ipfs-files"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	ipfscore "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/core"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/keyfs"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	coreoptions "github.com/ipfs/interface-go-ipfs-core/options"
)

var _ fuselib.FileSystemInterface = (*FileSystem)(nil)

var errKeyIsNotDir = errors.New("key root is not a directory")

type FileSystem struct {
	fusecom.SharedMethods
	provcom.IPFSCore

	initChan fusecom.InitSignal

	// maps keys to a single shared I/O instance for that key
	mfsTable *mfsWrapper
	uioTable *keyfs.FileWrapper

	// maps handles to an I/O wrapper that looks unique but uses the same underlying I/O
	files       fusecom.FileTable
	directories fusecom.DirectoryTable

	log  logging.EventLogger
	ipns fuselib.FileSystemInterface
}

func NewFileSystem(ctx context.Context, core coreiface.CoreAPI, opts ...Option) *FileSystem {
	settings := parseOptions(opts...)

	return &FileSystem{
		IPFSCore: provcom.NewIPFSCore(ctx, core, settings.ResourceLock),
		initChan: settings.InitSignal,
		log:      settings.Log,
	}
}
func validatePath(path string) error {
	if path == "" {
		return errors.New("empty path")
	}

	if path[0] != '/' {
		return errors.New("path does not begin with /")
	}

	return nil
}

func splitPath(path string) (namespace, remainder string) {
	slashIndex := 1 // skip leading slash
	slashIndex += strings.IndexRune(path[1:], '/')

	if slashIndex == 0 { // input looks like: `/namespace`
		namespace = path[1:]
		remainder = path[0:1]
	} else { // input looks like: `/namespace/sub...`
		namespace = path[1:slashIndex]
		remainder = path[slashIndex:]
	}
	return
}

func checkAndSplitPath(path string) (string, string, error) {
	if err := validatePath(path); err != nil {
		return "", "", err
	}

	namespace, remainder := splitPath(path)
	return namespace, remainder, nil
}

func (fs *FileSystem) getMFSRoot(coreKey coreiface.Key) (fuselib.FileSystemInterface, error) {
	iStat, _, err := transform.GetAttr(fs.Ctx(), coreKey.Path(), fs.Core(), transform.IPFSStatRequest{Type: true})
	if err != nil {
		return nil, err
	}
	if iStat.FileType != coreiface.TDirectory {
		return nil, errKeyIsNotDir
	}

	mfs, err := fs.mfsTable.OpenRoot(coreKey)
	if err != nil {
		return nil, err
	}
	return mfs, nil
}

func offlineAPI(core coreiface.CoreAPI) coreiface.CoreAPI {
	oAPI, err := core.WithOptions(coreoptions.Api.Offline(true))
	if err != nil {
		panic(err)
	}
	return oAPI
}

// caller should expect key to be nil if not found, with err also being nil
func checkKey(ctx context.Context, keyAPI coreiface.KeyAPI, keyName string) (coreiface.Key, error) {
	callContext, cancel := context.WithCancel(ctx)
	defer cancel()

	keys, err := keyAPI.List(callContext)
	if err != nil {
		return nil, err
	}

	for _, key := range keys {
		if key.Name() == keyName {
			return key, nil
		}
	}

	// not having a key is not an error
	return nil, nil
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
			if retErr != nil {
				c <- retErr
			}
			close(c)
		}

		fs.log.Debugf("init finished")
	}()

	// proxy non-key subrequests to IPNS
	initChan := make(fusecom.InitSignal)
	ipnsSubsys := ipfscore.NewFileSystem(fs.Ctx(), fs.Core(),
		ipfscore.WithNamespace(mountinter.NamespaceIPNS),
		ipfscore.WithCommon(
			fusecom.WithInitSignal(initChan),
			fusecom.WithParent(fs),
			fusecom.WithResourceLock(fs.IPFSCore),
		),
	)

	go ipnsSubsys.Init()
	for err := range initChan {
		if err != nil {
			fs.log.Errorf("subsystem init failed:%s", err)
			retErr = err // last err returned but all logged
		}
	}

	fs.ipns = ipnsSubsys

	// fs.mountTime = fuselib.Now()
	fs.files = fusecom.NewFileTable()
	fs.directories = fusecom.NewDirectoryTable()

	fs.mfsTable = newRootTable(fs.Ctx(), fs.Core())
	fs.uioTable = keyfs.NewFileWrapper(fs.Ctx(), fs.Core())
}

func (fs *FileSystem) Destroy() {
	fs.log.Debugf("Destroy - Requested")
}

func (fs *FileSystem) Getattr(path string, stat *fuselib.Stat_t, fh uint64) (errc int) {
	fs.log.Debugf("Getattr - {%X}%q", fh, path)

	keyName, remainder, err := checkAndSplitPath(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT
	}
	if keyName == "" { // root request
		stat.Mode = fuselib.S_IFDIR
		fusecom.ApplyPermissions(true, &stat.Mode)
		stat.Uid, stat.Gid, _ = fuselib.Getcontext()
		return fusecom.OperationSuccess
	}

	coreKey, err := checkKey(fs.Ctx(), fs.Core().Key(), keyName)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT
	}
	if coreKey != nil { // we own this key; stat it and mark it as writable
		iStat, _, err := transform.GetAttr(fs.Ctx(), coreKey.Path(), fs.Core(), transform.IPFSStatRequestAll)
		if err != nil {
			fs.log.Error(err)
			return -fuselib.ENOENT
		}

		if remainder == "/" { // if no subpath, we're already done
			*stat = *iStat.ToFuse()
			fusecom.ApplyPermissions(true, &stat.Mode)
			stat.Uid, stat.Gid, _ = fuselib.Getcontext()
			return fusecom.OperationSuccess
		}

		if iStat.FileType != coreiface.TDirectory {
			err := fmt.Errorf("%q requested but %q is not a directory", path, keyName)
			fs.log.Error(err)
			// TODO [general] we also need to return this when someone requests "/file/" instead of "/file"
			return -fuselib.ENOTDIR
		}

		mfs, err := fs.mfsTable.OpenRoot(coreKey)
		if err != nil {
			fs.log.Error(err)
			return -fuselib.EIO
		}
		return mfs.Getattr(remainder, stat, fh)
	}

	// proxy the request to ipns if we don't own the key
	return fs.ipns.Getattr(remainder, stat, fh)
}

func (fs *FileSystem) Opendir(path string) (int, uint64) {
	fs.log.Debugf("Opendir - %q", path)

	keyName, remainder, err := checkAndSplitPath(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}
	if keyName == "" { // root request
		keyDir, err := keyfs.OpenDir(fs.Ctx(), fs.Core())
		if err != nil {
			return -fuselib.ENOENT, fusecom.ErrorHandle
		}
		handle, err := fs.directories.Add(keyDir)
		if err != nil { // TODO: inspect/transform error
			fs.log.Error(err)
			return -fuselib.EMFILE, fusecom.ErrorHandle
		}
		return fusecom.OperationSuccess, handle
	}

	coreKey, err := checkKey(fs.Ctx(), fs.Core().Key(), keyName)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}
	if coreKey != nil { // we own this key
		mfs, err := fs.getMFSRoot(coreKey)
		if err != nil {
			if err == errKeyIsNotDir {
				fs.log.Errorf("%q requested but %q is not a directory", path, keyName)
				return -fuselib.ENOTDIR, fusecom.ErrorHandle
			}
			fs.log.Error(err)
			return -fuselib.ENOENT, fusecom.ErrorHandle
		}
		return mfs.Opendir(remainder)
	}

	return fs.ipns.Opendir(remainder)
}

func (fs *FileSystem) Releasedir(path string, fh uint64) int {
	fs.log.Debugf("Releasedir - {%X}%q", fh, path)

	keyName, remainder, err := checkAndSplitPath(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EBADF
	}

	if keyName == "" { // root request
		goErr, errNo := fusecom.ReleaseDir(fs.directories, fh)
		if goErr != nil {
			fs.log.Error(goErr)
		}

		return errNo
	}

	coreKey, err := checkKey(fs.Ctx(), fs.Core().Key(), keyName)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EBADF
	}

	if coreKey != nil { // we own this key
		mfs, err := fs.getMFSRoot(coreKey)
		if err != nil {
			fs.log.Error(err)
			return -fuselib.EBADF
		}
		return mfs.Releasedir(remainder, fh)
	}

	return fs.ipns.Releasedir(remainder, fh)
}

func (fs *FileSystem) Readdir(path string,
	fill func(name string, stat *fuselib.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) int {
	fs.log.Debugf("Readdir - {%X|%d}%q", fh, ofst, path)

	keyName, remainder, err := checkAndSplitPath(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EBADF
	}

	if keyName == "" { // root request
		directory, err := fs.directories.Get(fh)
		if err != nil {
			fs.log.Error(err)
			return -fuselib.EBADF
		}

		goErr, errNo := fusecom.FillDir(fs.Ctx(), directory, false, fill, ofst)
		if goErr != nil {
			fs.log.Error(goErr)
		}

		return errNo
	}

	coreKey, err := checkKey(fs.Ctx(), fs.Core().Key(), keyName)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EBADF
	}

	if coreKey != nil { // we own this key
		mfs, err := fs.getMFSRoot(coreKey)
		if err != nil {
			fs.log.Error(err)
			return -fuselib.EBADF
		}
		return mfs.Readdir(remainder, fill, ofst, fh)
	}

	return fs.ipns.Readdir(remainder, fill, ofst, fh)
}

func (fs *FileSystem) Open(path string, flags int) (int, uint64) {
	fs.log.Debugf("Open - {%X}%q", flags, path)

	goErr, errNo := fusecom.CheckOpenFlagsBasic(true, flags)
	if goErr != nil {
		fs.log.Error(goErr)
		return errNo, fusecom.ErrorHandle
	}

	keyName, remainder, err := checkAndSplitPath(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}

	if keyName == "" { // root request
		fs.log.Error(fuselib.Error(-fuselib.EISDIR))
		return -fuselib.EISDIR, fusecom.ErrorHandle
	}

	coreKey, err := checkKey(fs.Ctx(), fs.Core().Key(), keyName)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}

	if coreKey != nil { // we own this key
		if remainder == "/" { // request for the key itself, as a file
			// make sure the key is actually a file
			iStat, _, err := transform.GetAttr(fs.Ctx(), coreKey.Path(), fs.Core(), transform.IPFSStatRequest{Type: true})
			if err != nil {
				fs.log.Error(err)
				return -fuselib.ENOENT, fusecom.ErrorHandle
			}
			if iStat.FileType != coreiface.TFile {
				fs.log.Errorf("%q requested but %q is not a file", path, keyName)
				// TODO [general] we also need to return this when someone requests "/file/" instead of "/file"
				return -fuselib.EISDIR, fusecom.ErrorHandle
			}

			// if it is, open it and assign it a handle
			keyFile, err := fs.uioTable.Open(coreKey, transform.IOFlagsFromFuse(flags))
			if err != nil {
				fs.log.Error(err)
				return -fuselib.ENOENT, fusecom.ErrorHandle
			}
			handle, err := fs.files.Add(keyFile)
			if err != nil {
				fs.log.Error(err)
				return -fuselib.EMFILE, fusecom.ErrorHandle
			}
			return fusecom.OperationSuccess, handle
		}

		// request for a path within the key (as a directory)
		mfs, err := fs.mfsTable.OpenRoot(coreKey)
		if err != nil {
			if err == errKeyIsNotDir {
				fs.log.Errorf("%q requested but %q is not a directory", path, keyName)
				return -fuselib.ENOTDIR, fusecom.ErrorHandle
			}
			fs.log.Error(err)
			return -fuselib.ENOENT, fusecom.ErrorHandle
		}
		return mfs.Open(remainder, flags)
	}

	// request for something we don't own, relay it to ipns
	return fs.ipns.Open(remainder, flags)
}

func (fs *FileSystem) Release(path string, fh uint64) int {
	fs.log.Debugf("Release - {%X}%q", fh, path)

	keyName, remainder, err := checkAndSplitPath(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EBADF
	}

	if keyName == "" { // root request; we're never a file
		fs.log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	coreKey, err := checkKey(fs.Ctx(), fs.Core().Key(), keyName)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EBADF
	}

	if coreKey != nil { // we own this key
		if remainder == "/" { // request for the key itself
			goErr, errNo := fusecom.ReleaseFile(fs.files, fh)
			if goErr != nil {
				fs.log.Error(goErr)
			}
			return errNo
		}

		// request for a path within the key (as a directory)
		mfs, err := fs.getMFSRoot(coreKey)
		if err != nil {
			fs.log.Error(err)
			return -fuselib.EBADF
		}
		return mfs.Release(remainder, fh)
	}

	return fs.ipns.Release(remainder, fh)
}

func (fs *FileSystem) Read(path string, buff []byte, ofst int64, fh uint64) int {
	fs.log.Debugf("Read - {%X}%q", fh, path)

	keyName, remainder, err := checkAndSplitPath(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EBADF
	}

	if keyName == "" { // root request; we're never a file
		fs.log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	coreKey, err := checkKey(fs.Ctx(), fs.Core().Key(), keyName)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EBADF
	}

	if coreKey != nil { // we own this key
		if remainder == "/" { // request for the key itself
			file, err := fs.files.Get(fh)
			if err != nil {
				fs.log.Error(fuselib.Error(-fuselib.EBADF))
				return -fuselib.EBADF
			}

			err, retVal := fusecom.ReadFile(file, buff, ofst)
			if err != nil && err != io.EOF {
				fs.log.Error(err)
			}
			return retVal
		}

		mfs, err := fs.getMFSRoot(coreKey)
		if err != nil {
			fs.log.Error(err)
			return -fuselib.EBADF
		}
		return mfs.Read(remainder, buff, ofst, fh)
	}

	return fs.ipns.Read(remainder, buff, ofst, fh)
}

func (fs *FileSystem) Readlink(path string) (int, string) {
	fs.log.Debugf("Readlink - %q", path)

	keyName, remainder, err := checkAndSplitPath(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT, ""
	}

	if keyName == "" { // root request
		fs.log.Warnf("Readlink - root path is an invalid request")
		return -fuse.EINVAL, ""
	}

	coreKey, err := checkKey(fs.Ctx(), fs.Core().Key(), keyName)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT, ""
	}

	if coreKey != nil { // we own this key
		if remainder == "/" { // request for the key itself, as a link
			// make sure the key is actually a link
			iStat, _, err := transform.GetAttr(fs.Ctx(), coreKey.Path(), fs.Core(), transform.IPFSStatRequest{Type: true})
			if err != nil {
				fs.log.Error(err)
				return -fuselib.ENOENT, ""
			}

			if iStat.FileType != coreiface.TSymlink {
				fs.log.Errorf("%q requested but %q is not a link", path, keyName)
				return -fuselib.EINVAL, ""
			}

			// if it is, read it
			linkNode, err := fs.Core().Unixfs().Get(fs.Ctx(), coreKey.Path())
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

		// request for a path within the key (as a directory)
		mfs, err := fs.mfsTable.OpenRoot(coreKey)
		if err != nil {
			if err == errKeyIsNotDir {
				fs.log.Errorf("%q requested but %q is not a directory", path, keyName)
				return -fuselib.ENOTDIR, ""
			}
			fs.log.Error(err)
			return -fuselib.ENOENT, ""
		}
		return mfs.Readlink(remainder)
	}

	// request for something we don't own, relay it to ipns
	return fs.ipns.Readlink(remainder)
}
