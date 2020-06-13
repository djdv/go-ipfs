package core

import (
	"context"
	"io"
	gopath "path"
	"path/filepath"
	"time"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/empty"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/ipfscore"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

const filesWritable = false

type FileSystem struct {
	fusecom.SharedMethods                     // FUSE interface stubs
	intf                  transform.Interface // interface between FUSE and IPFS core

	initChan fusecom.InitSignal // optional message channel to communicate with the caller

	files       fusecom.FileTable // reference tables
	directories fusecom.DirectoryTable

	// if readdirplus is enable
	// we'll use this function to equip directories with a means to stat their elements
	// using a different method depending on the namespace we're operating on
	readdirPlusGen func(transform.Interface, string, *fuselib.Stat_t) fusecom.StatFunc

	log logging.EventLogger

	mountTimeGroup fusecom.StatTimeGroup // artificial file time signatures
	rootStat       *fuselib.Stat_t       // artificial root attributes
}

func NewFileSystem(ctx context.Context, core coreiface.CoreAPI, opts ...Option) *FileSystem {
	settings := parseOptions(opts...)

	if settings.namespace == mountinter.NamespaceNone {
		settings.namespace = mountinter.NamespaceCore
	}

	fs := &FileSystem{
		intf:     ipfscore.NewInterface(ctx, core, settings.namespace),
		initChan: settings.InitSignal,
		log:      settings.Log,
	}

	if provcom.CanReaddirPlus {
		if settings.namespace == mountinter.NamespaceIPFS {
			fs.readdirPlusGen = staticStat
		} else {
			fs.readdirPlusGen = dynamicStat
		}
	}

	return fs
}

func (fs *FileSystem) Init() {
	fs.log.Debug("init")
	defer func() {
		if fs.initChan != nil {
			close(fs.initChan)
		}
		fs.log.Debugf("init finished")
	}()

	fs.files = fusecom.NewFileTable()
	fs.directories = fusecom.NewDirectoryTable()

	timeOfMount := fuselib.Now()

	fs.mountTimeGroup = fusecom.StatTimeGroup{
		Atim:     timeOfMount,
		Mtim:     timeOfMount,
		Ctim:     timeOfMount,
		Birthtim: timeOfMount,
	}

	fs.rootStat = &fuselib.Stat_t{
		Mode:     fuselib.S_IFDIR | (fusecom.IRXA &^ fuselib.S_IXOTH), // |0554
		Atim:     timeOfMount,
		Mtim:     timeOfMount,
		Ctim:     timeOfMount,
		Birthtim: timeOfMount,
	}
}

func (fs *FileSystem) Destroy() {
	fs.log.Debugf("Destroy - Requested")
}

func (fs *FileSystem) Getattr(path string, stat *fuselib.Stat_t, fh uint64) int {
	fs.log.Debugf("Getattr - {%X}%q", fh, path)

	switch path {
	case "":
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT

	case "/":
		*stat = *fs.rootStat
		stat.Uid, stat.Gid, _ = fuselib.Getcontext()
		return fusecom.OperationSuccess

	default:
		iStat, _, err := fs.intf.Info(path, transform.IPFSStatRequestAll)
		if err != nil {
			errNo := fusecom.InterpretError(err)
			if errNo != -fuselib.ENOENT { // don't flood the logs with "not found" errors
				fs.log.Error(err)
			}
			return errNo
		}

		var ids fusecom.StatIDGroup
		ids.Uid, ids.Gid, _ = fuselib.Getcontext()
		fusecom.ApplyIntermediateStat(stat, iStat)
		fusecom.ApplyCommonsToStat(stat, filesWritable, fs.mountTimeGroup, ids)
		return fusecom.OperationSuccess
	}
}

func (fs *FileSystem) Opendir(path string) (int, uint64) {
	fs.log.Debugf("Opendir - %q", path)

	if path == "" { // invalid requests
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}

	// FIXME:
	// on Windows, (specifically in Powerhshell) when mounted to a UNC path
	// operations like `Get-ChildItem "\\servername\share\Qm..."` work fine, but
	// `Set-Location "\\servername\share\Qm..."` always fail
	// this seems to do with the fact the share's root does not actually contain the target
	// (pwsh seems to read the root to verify existence before attempting to changing into it)
	// the same behaviour is not present when mounted to a drivespec like `I:`
	// or in other applications (namely Explorer)
	// We could probably fix this by caching the first component of the last getattr call
	// and `fill`ing it in during Readdir("/")
	// failing this, a more persistant LRU cache could be shown in the root

	var directory transform.Directory
	if path == "/" { // root requests
		directory = empty.OpenDir()
	} else { // sub requests
		var err error
		directory, err = fs.intf.OpenDirectory(path)
		if err != nil {
			fs.log.Error(err)
			return fusecom.InterpretError(err), fusecom.ErrorHandle
		}

		if provcom.CanReaddirPlus {
			// NOTE: we won't have access to the fuse context in `Readdir` (depending on the fuse implementation)
			// so we associate IDs with the caller who opened the directory
			var ids fusecom.StatIDGroup
			ids.Uid, ids.Gid, _ = fuselib.Getcontext()
			templateStat := new(fuselib.Stat_t)
			fusecom.ApplyCommonsToStat(templateStat, filesWritable, fs.mountTimeGroup, ids)

			directory = fusecom.UpgradeDirectory(directory, fs.readdirPlusGen(fs.intf, path, templateStat))
		}
	}

	handle, err := fs.directories.Add(directory)
	if err != nil { // TODO: transform error
		fs.log.Error(fuselib.Error(-fuselib.EMFILE))
		return -fuselib.EMFILE, fusecom.ErrorHandle
	}

	return fusecom.OperationSuccess, handle
}

func getStat(r transform.Interface, path string, template *fuselib.Stat_t) *fuselib.Stat_t {
	iStat, _, err := r.Info(path, transform.IPFSStatRequestAll)
	if err != nil {
		return nil
	}

	subStat := new(fuselib.Stat_t)
	*subStat = *template
	fusecom.ApplyIntermediateStat(subStat, iStat)
	return subStat
}

// statticStat generates a StatFunc
// that fetches attributes for a requests, and caches the results for subsiquent requests
func staticStat(r transform.Interface, basePath string, template *fuselib.Stat_t) fusecom.StatFunc {
	stats := make(map[string]*fuselib.Stat_t, 1)

	return func(name string) *fuselib.Stat_t {
		if cachedStat, ok := stats[name]; ok {
			return cachedStat
		}

		subStat := getStat(r, gopath.Join(basePath, name), template)
		stats[name] = subStat
		return subStat
	}
}

// dynamicStat generates a StatFunc
// that always fetches attributes for a requests (assuming they may have changed since the last request)
func dynamicStat(r transform.Interface, basePath string, template *fuselib.Stat_t) fusecom.StatFunc {
	return func(name string) *fuselib.Stat_t {
		return getStat(r, gopath.Join(basePath, name), template)
	}
}

func (fs *FileSystem) Releasedir(path string, fh uint64) int {
	fs.log.Debugf("Releasedir - {%X}%q", fh, path)

	errNo, err := fusecom.ReleaseDir(fs.directories, fh)
	if err != nil {
		fs.log.Error(err)
	}

	return errNo
}

func (fs *FileSystem) Readdir(path string,
	fill func(name string, stat *fuselib.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) int {
	fs.log.Debugf("Readdir - {%X|%d}%q", fh, ofst, path)

	if fh == fusecom.ErrorHandle {
		fs.log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	directory, err := fs.directories.Get(fh)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	// TODO: change this context; needs parent
	callCtx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
	defer cancel()

	errNo, err := fusecom.FillDir(callCtx, directory, fill, ofst)
	if err != nil {
		fs.log.Error(err)
	}

	return errNo
}

func (fs *FileSystem) Open(path string, flags int) (int, uint64) {
	fs.log.Debugf("Open - {%X}%q", flags, path)

	errNo, err := fusecom.CheckOpenFlagsBasic(filesWritable, flags)
	if err != nil {
		fs.log.Error(err)
		return errNo, fusecom.ErrorHandle
	}

	errNo, err = fusecom.CheckOpenPathBasic(path)
	if err != nil {
		fs.log.Error(err)
		return errNo, fusecom.ErrorHandle
	}

	file, err := fs.intf.Open(path, fusecom.IOFlagsFromFuse(flags))
	if err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err), fusecom.ErrorHandle
	}

	handle, err := fs.files.Add(file)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.EMFILE))
		return -fuselib.EMFILE, fusecom.ErrorHandle
	}

	return fusecom.OperationSuccess, handle
}

func (fs *FileSystem) Release(path string, fh uint64) int {
	fs.log.Debugf("Release - {%X}%q", fh, path)

	errNo, err := fusecom.ReleaseFile(fs.files, fh)
	if err != nil {
		fs.log.Error(err)
	}

	return errNo
}

func (fs *FileSystem) Read(path string, buff []byte, ofst int64, fh uint64) int {
	fs.log.Debugf("Read - Request {%X|%d}%q", fh, ofst, path)

	// TODO: [review] we need to do things on failure
	// the OS typically triggers a close, but we shouldn't expect it to invalidate this record for us
	// we also might want to store a file cursor to reduce calls to seek
	// the same thing already happens internally so it's at worst the overhead of a call right now

	file, err := fs.files.Get(fh)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	retVal, err := fusecom.ReadFile(file, buff, ofst)
	if err != nil && err != io.EOF {
		fs.log.Error(err)
	}
	return retVal
}

func (fs *FileSystem) Readlink(path string) (int, string) {
	fs.log.Debugf("Readlink - %q", path)

	switch path {
	case "/":
		fs.log.Warnf("Readlink - root path is an invalid request")
		return -fuselib.EINVAL, ""

	case "":
		fs.log.Error("Readlink - empty request")
		return -fuselib.ENOENT, ""
	}

	linkString, err := fs.intf.ExtractLink(path)
	if err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err), ""
	}

	// NOTE: paths returned here get sent back to the FUSE library
	// they should not be native paths, regardless of their source format
	return fusecom.OperationSuccess, filepath.ToSlash(linkString)
}
