package keyfs

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	ipfscore "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/core"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	coretransform "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/ipfscore"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/keyfs"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	coreoptions "github.com/ipfs/interface-go-ipfs-core/options"
)

var _ fuselib.FileSystemInterface = (*FileSystem)(nil)

const filesWritable = true

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

	mountTimeGroup fusecom.StatTimeGroup
	rootStat       *fuselib.Stat_t
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
		return nil, keyfs.ErrKeyIsNotDir
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

	timeOfMount := fuselib.Now()

	fs.mountTimeGroup = fusecom.StatTimeGroup{
		Atim:     timeOfMount,
		Mtim:     timeOfMount,
		Ctim:     timeOfMount,
		Birthtim: timeOfMount,
	}

	fs.rootStat = &fuselib.Stat_t{
		Mode: fuselib.S_IFDIR | fusecom.IRWXA&^(fuselib.S_IWOTH|fuselib.S_IXOTH), // |0774
		Atim: timeOfMount,
		Mtim: timeOfMount,
		Ctim: timeOfMount,
	}
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
		*stat = *fs.rootStat
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
			var ids fusecom.StatIDGroup
			ids.Uid, ids.Gid, _ = fuselib.Getcontext()
			fusecom.ApplyCommonsToStat(stat, filesWritable, fs.mountTimeGroup, ids)
			return fusecom.OperationSuccess
		}

		if iStat.FileType != coreiface.TDirectory {
			if iStat.FileType == coreiface.TSymlink { // log flood prevention
				fs.log.Warnf("%q requested but %q is not a directory (type: %s)", path, keyName, iStat.FileType.String())
			} else {
				fs.log.Errorf("%q requested but %q is not a directory (type: %s)", path, keyName, iStat.FileType.String())
			}
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
	return fs.ipns.Getattr(path, stat, fh)
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

	if coreKey != nil { // we own this key; intercept request
		mfs, err := fs.getMFSRoot(coreKey)
		if err != nil {
			if err == keyfs.ErrKeyIsNotDir {
				fs.log.Errorf("%q requested but %q is not a directory", path, keyName)
				return -fuselib.ENOTDIR, fusecom.ErrorHandle
			}
			fs.log.Error(err)
			return -fuselib.ENOENT, fusecom.ErrorHandle
		}
		return mfs.Opendir(remainder)
	}

	return fs.ipns.Opendir(path) // pass through full path
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

	// request for a path within the key (as a directory)
	coreKey, err := checkKey(fs.Ctx(), fs.Core().Key(), keyName)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EBADF
	}

	// FIXME: if a key is removed via rmdir, the coreKey will be nil when we reach here
	// this is because the sequence is opendir; rmdir; closedir (on this machine)
	// so we'll never try to remove it from the mfs table
	// because the key is already removed from the keystore
	// this then passes the release request to ipns which is BAD
	// we'll need to consider some way to handle this
	// same in the Release() path
	// maybe store a list of active keys on the fs and check that before checking the keystore
	if coreKey != nil {
		mfs, err := fs.getMFSRoot(coreKey)
		if err != nil {
			fs.log.Error(err)
			return -fuselib.EBADF
		}
		return mfs.Releasedir(remainder, fh)
	}

	return fs.ipns.Releasedir(path, fh)
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

		goErr, errNo := fusecom.FillDir(fs.Ctx(), directory, fill, ofst)
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

	if coreKey != nil {
		mfs, err := fs.getMFSRoot(coreKey)
		if err != nil {
			fs.log.Error(err)
			return -fuselib.EBADF
		}
		return mfs.Readdir(remainder, fill, ofst, fh)
	}

	return fs.ipns.Readdir(path, fill, ofst, fh)
}

func (fs *FileSystem) Open(path string, flags int) (int, uint64) {
	fs.log.Debugf("Open - {%X}%q", flags, path)
	return fs.open(path, flags)
}

func (fs *FileSystem) open(path string, flags int) (int, uint64) {
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
			if err == keyfs.ErrKeyIsNotDir {
				fs.log.Errorf("%q requested but %q is not a directory", path, keyName)
				return -fuselib.ENOTDIR, fusecom.ErrorHandle
			}
			fs.log.Error(err)
			return -fuselib.ENOENT, fusecom.ErrorHandle
		}
		return mfs.Open(remainder, flags)
	}

	// request for something we don't own, relay it to ipns
	return fs.ipns.Open(path, flags)
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

	if remainder == "/" { // key request
		goErr, errNo := fusecom.ReleaseFile(fs.files, fh)
		if goErr != nil {
			fs.log.Error(goErr)
		}
		return errNo
	}

	// request for a path within the key (as a directory)
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
		return mfs.Release(remainder, fh)
	}

	return fs.ipns.Release(path, fh)
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

	return fs.ipns.Read(path, buff, ofst, fh)
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
		return -fuselib.EINVAL, ""
	}

	coreKey, err := checkKey(fs.Ctx(), fs.Core().Key(), keyName)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT, ""
	}

	if coreKey != nil { // we own this key
		if remainder == "/" { // request for the key itself, as a link
			linkString, err := coretransform.Readlink(fs.Ctx(), coreKey.Path(), fs.Core())
			if err != nil {
				fs.log.Error(err)
				return err.ToFuse(), ""
			}

			// NOTE: paths returned here get sent back to the FUSE library
			// they should not be native paths, regardless of their source format
			return fusecom.OperationSuccess, filepath.ToSlash(linkString)
		}

		// request for a path within the key (as a directory)
		mfs, err := fs.mfsTable.OpenRoot(coreKey)
		if err != nil {
			if err == keyfs.ErrKeyIsNotDir {
				fs.log.Errorf("%q requested but %q is not a directory", path, keyName)
				return -fuselib.ENOTDIR, ""
			}
			fs.log.Error(err)
			return -fuselib.ENOENT, ""
		}
		return mfs.Readlink(remainder)
	}

	// request for something we don't own, relay it to ipns
	return fs.ipns.Readlink(path)
}

func (fs *FileSystem) Create(path string, flags int, mode uint32) (int, uint64) {
	fs.log.Warnf("Create - {%X|%X}%q", flags, mode, path)

	// TODO: why is fuselib passing us flags and what are they?
	// both FUSE and SUS predefine what they should be (to Open)

	//return fs.open(path, fuselib.O_WRONLY|fuselib.O_CREAT|fuselib.O_TRUNC)

	// disabled until we parse relevant flags in open
	// fuse will do shenanigans to make this work
	return -fuselib.ENOSYS, fusecom.ErrorHandle
}

func (fs *FileSystem) makeEmpty(path string, mode uint32, filetype coreiface.FileType) int {
	keyName, remainder, err := checkAndSplitPath(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT
	}

	if keyName == "" { // root request
		fs.log.Error(fuselib.Error(-fuselib.EEXIST))
		return -fuselib.EEXIST
	}

	coreKey, err := checkKey(fs.Ctx(), fs.Core().Key(), keyName)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EIO
	}

	if coreKey != nil {
		if remainder == "/" { // request to make the key itself; deny this since it exists
			fs.log.Error(fuselib.Error(-fuselib.EEXIST))
			return -fuselib.EEXIST
		}

		// request for a path within the key (as a directory)
		mfs, err := fs.mfsTable.OpenRoot(coreKey)
		if err != nil {
			if err == keyfs.ErrKeyIsNotDir {
				fs.log.Errorf("%q requested but %q is not a directory", path, keyName)
				return -fuselib.ENOTDIR
			}
			fs.log.Error(err)
			return -fuselib.ENOENT
		}
		if filetype == coreiface.TFile {
			// MAGIC: 0; device id has no meaning to MFS
			return mfs.Mknod(remainder, mode, 0)
		}
		return mfs.Mkdir(path, mode)
	}

	if remainder == "/" { // request for the key itself, which doesn't already exist; make it
		var err error
		if filetype == coreiface.TFile {
			err = keyfs.Mknod(fs.Ctx(), fs.Core(), keyName)
		} else {
			err = keyfs.Mkdir(fs.Ctx(), fs.Core(), keyName)
		}

		if err != nil {
			fs.log.Error(err)
			return -fuselib.EIO
		}
		return fusecom.OperationSuccess
	}

	// subrequest for a key that doesn't exist
	return -fuselib.ENOENT
}

func (fs *FileSystem) Mknod(path string, mode uint32, dev uint64) int {
	fs.log.Debugf("Mknod - Request {%X|%d}%q", mode, dev, path)
	return fs.makeEmpty(path, mode, coreiface.TFile)
}

func (fs *FileSystem) Truncate(path string, size int64, fh uint64) int {
	fs.log.Debugf("Truncate - Request {%X|%d}%q", fh, size, path)

	if size < 0 {
		return -fuselib.EINVAL
	}

	keyName, remainder, err := checkAndSplitPath(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EBADF
	}

	if keyName == "" { // root request; we're never a file
		fs.log.Error(fuselib.Error(-fuselib.EISDIR))
		return -fuselib.EISDIR
	}

	coreKey, err := checkKey(fs.Ctx(), fs.Core().Key(), keyName)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EBADF
	}

	if coreKey != nil { // we own this key
		if remainder == "/" { // request for the key itself
			file, err := fs.files.Get(fh)
			if err == nil { // if the file handle is valid, truncate the file directly
				if err := file.Truncate(uint64(size)); err != nil {
					fs.log.Error(err)
					return -fuselib.EIO
				}
			}

			// otherwise fallback to slow path truncate
			if err := fs.uioTable.Truncate(coreKey, uint64(size)); err != nil {
				// TODO: [SUS compliance] disambiguate errors
				return -fuselib.EIO
			}
			return fusecom.OperationSuccess
		}

		// subrequest, hand off to mfs
		mfs, err := fs.getMFSRoot(coreKey)
		if err != nil {
			if err == keyfs.ErrKeyIsNotDir {
				fs.log.Errorf("%q requested but %q is not a directory", path, keyName)
				return -fuselib.ENOTDIR
			}
			fs.log.Error(err)
			return -fuselib.ENOENT
		}
		return mfs.Truncate(remainder, size, fh)
	}

	// request for a key that doesn't exist
	return -fuselib.ENOENT
}

func (fs *FileSystem) Write(path string, buff []byte, ofst int64, fh uint64) int {
	fs.log.Debugf("Write - Request {%X|%d|%d}%q", fh, len(buff), ofst, path)

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

			err, retVal := fusecom.WriteFile(file, buff, ofst)
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
		return mfs.Write(remainder, buff, ofst, fh)
	}

	// request for a key that doesn't exist
	return -fuselib.ENOENT
}

func (fs *FileSystem) Link(oldpath string, newpath string) int {
	fs.log.Warnf("Link - Request %q<->%q", oldpath, newpath)
	return -fuselib.ENOSYS
}

func (fs *FileSystem) Unlink(path string) int {
	fs.log.Debugf("Unlink - Request %q", path)

	keyName, remainder, err := checkAndSplitPath(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT
	}

	if keyName == "" { // root request
		fs.log.Error(fuselib.Error(-fuselib.EPERM))
		return -fuselib.EPERM
	}

	coreKey, err := checkKey(fs.Ctx(), fs.Core().Key(), keyName)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EIO
	}

	if coreKey == nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}

	if remainder == "/" { // request to remove the key itself
		err := keyfs.Unlink(fs.Ctx(), fs.Core(), coreKey)
		var errNo int
		switch err {
		case nil:
			errNo = fusecom.OperationSuccess

		case keyfs.ErrKeyIsNotFile:
			errNo = -fuselib.EPERM

		default:
			errNo = -fuselib.ENOENT
		}

		if errNo != fusecom.OperationSuccess {
			fs.log.Error(err)
		}
		return errNo
	}

	// request for a path within the key (as a directory)
	mfs, err := fs.mfsTable.OpenRoot(coreKey)
	if err != nil {
		if err == keyfs.ErrKeyIsNotDir {
			fs.log.Errorf("%q requested but %q is not a directory", path, keyName)
			return -fuselib.ENOTDIR
		}
		fs.log.Error(err)
		return -fuselib.ENOENT
	}

	return mfs.Unlink(remainder)
}

func (fs *FileSystem) Mkdir(path string, mode uint32) int {
	fs.log.Debugf("Mkdir - Request {%X}%q", mode, path)
	return fs.makeEmpty(path, mode, coreiface.TDirectory)
}

func (fs *FileSystem) Rmdir(path string) int {
	fs.log.Debugf("Rmdir - Request %q", path)

	keyName, remainder, err := checkAndSplitPath(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT
	}

	if keyName == "" { // root request
		// TODO: [review] is this the most appropriate error?
		fs.log.Error(fuselib.Error(-fuselib.EBUSY))
		return -fuselib.EBUSY
	}

	coreKey, err := checkKey(fs.Ctx(), fs.Core().Key(), keyName)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EIO
	}

	if coreKey == nil {
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT
	}

	if remainder == "/" { // request to remove the key itself
		if err := keyfs.Rmdir(fs.Ctx(), fs.Core(), coreKey); err != nil {
			fs.log.Error(err)
			return err.ToFuse()
		}
		return fusecom.OperationSuccess
	}

	// request for a path within the key (as a directory)
	mfs, err := fs.mfsTable.OpenRoot(coreKey)
	if err != nil {
		if err == keyfs.ErrKeyIsNotDir {
			fs.log.Errorf("%q requested but %q is not a directory", path, keyName)
			return -fuselib.ENOTDIR
		}
		fs.log.Error(err)
		return -fuselib.ENOENT
	}

	return mfs.Rmdir(remainder)
}

func (fs *FileSystem) Symlink(target string, newpath string) int {
	fs.log.Debugf("Symlink - Request %q->%q", newpath, target)

	keyName, remainder, err := checkAndSplitPath(newpath)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT
	}

	if keyName == "" { // root request
		fs.log.Error(fuselib.Error(-fuselib.EEXIST))
		return -fuselib.EEXIST
	}

	coreKey, err := checkKey(fs.Ctx(), fs.Core().Key(), keyName)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EIO
	}

	if coreKey != nil {
		if remainder == "/" { // request to make the key itself; deny this since it exists
			fs.log.Error(fuselib.Error(-fuselib.EEXIST))
			return -fuselib.EEXIST
		}

		// request for a path within the key (as a directory)
		mfs, err := fs.mfsTable.OpenRoot(coreKey)
		if err != nil {
			if err == keyfs.ErrKeyIsNotDir {
				fs.log.Errorf("%q requested but %q is not a directory", newpath, keyName)
				return -fuselib.ENOTDIR
			}
			fs.log.Error(err)
			return -fuselib.ENOENT
		}
		return mfs.Symlink(target, remainder)
	}

	if remainder == "/" { // request for the key itself, which doesn't already exist; make it
		if err := keyfs.Symlink(fs.Ctx(), fs.Core(), keyName, target); err != nil {
			fs.log.Error(err)
			return -fuselib.EIO
		}
		return fusecom.OperationSuccess
	}

	// subrequest for a key that doesn't exist
	return -fuselib.ENOENT
}

func (fs *FileSystem) Rename(oldpath string, newpath string) int {
	fs.log.Warnf("Rename - Request %q->%q", oldpath, newpath)
	return -fuselib.ENOSYS
}
