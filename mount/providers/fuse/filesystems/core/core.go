package ipfscore

import (
	"context"
	"io"
	gopath "path"
	"path/filepath"
	"strings"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/empty"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/ipfscore"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

var _ fuselib.FileSystemInterface = (*FileSystem)(nil)

const filesWritable = false

type FileSystem struct {
	fusecom.SharedMethods
	provcom.IPFSCore

	initChan fusecom.InitSignal

	files       fusecom.FileTable
	directories fusecom.DirectoryTable

	log       logging.EventLogger
	namespace mountinter.Namespace

	mountTimeGroup fusecom.StatTimeGroup
	rootStat       *fuselib.Stat_t
}

func NewFileSystem(ctx context.Context, core coreiface.CoreAPI, opts ...Option) *FileSystem {
	settings := parseOptions(opts...)

	if settings.namespace == mountinter.NamespaceNone {
		settings.namespace = mountinter.NamespaceCore
	}

	return &FileSystem{
		IPFSCore:  provcom.NewIPFSCore(ctx, core, settings.ResourceLock),
		initChan:  settings.InitSignal,
		log:       settings.Log,
		namespace: settings.namespace,
	}
}

func (fs *FileSystem) joinRoot(path string) corepath.Path {
	return corepath.New(gopath.Join("/", strings.ToLower(fs.namespace.String()), path))
}

func (fs *FileSystem) Init() {
	fs.Lock()
	fs.log.Debug("init")
	defer func() {
		fs.Unlock()
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
		Mode:     fuselib.S_IFDIR | fusecom.IRXA&^(fuselib.S_IXOTH), // |0554
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
		if err := fs.getattr(path, stat); err != nil {
			// TODO: filter out "not found" if we can; omitted completley for now
			// fs.log.Error(err)
			return -fuselib.ENOENT
		}

		var ids fusecom.StatIDGroup
		ids.Uid, ids.Gid, _ = fuselib.Getcontext()
		fusecom.ApplyCommonsToStat(stat, filesWritable, fs.mountTimeGroup, ids)
		return fusecom.OperationSuccess
	}
}

// I hate this and hope the compiler can inline these
func (fs *FileSystem) getattr(path string, stat *fuselib.Stat_t) error {
	// expectation is to receive `/${multihash}`, not `${namespace}/${mulithash}`
	corePath := fs.joinRoot(path)
	return fs.coreGetattr(corePath, stat)
}
func (fs *FileSystem) coreGetattr(corePath corepath.Path, stat *fuselib.Stat_t) error {
	iStat, _, err := transform.GetAttr(fs.Ctx(), corePath, fs.Core(), transform.IPFSStatRequestAll)
	if err != nil {
		return err
	}

	if stat.Mode == 0 { // quick direct copy
		*stat = *iStat.ToFuse()
		return nil
	}

	// merge with pre-populated

	coreStat := iStat.ToFuse()

	stat.Mode &^= fuselib.S_IFMT // retain permissions bits, but clear the type bits
	stat.Mode |= coreStat.Mode   // we don't expect the core mode to contain anything other than the type bits, so we do no filtering on it
	stat.Size = coreStat.Size
	stat.Blksize = coreStat.Blksize
	stat.Blocks = coreStat.Blocks

	return nil
}

func (fs *FileSystem) Opendir(path string) (int, uint64) {
	fs.log.Debugf("Opendir - %q", path)

	if path == "" { // invalid requests
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}

	var directory transform.Directory

	if path == "/" { // root requests
		directory = empty.OpenDir()
		if provcom.CanReaddirPlus {
			// static assign a copy of the root template, adding in operation context values
			dotStat := new(fuselib.Stat_t)
			*dotStat = *fs.rootStat
			dotStat.Uid, dotStat.Gid, _ = fuselib.Getcontext()

			statFunc := func(name string) *fuselib.Stat_t {
				// TODO: if we're rooted under a parent, return it's stat for `..`
				if name == "." {
					return dotStat
				}
				return nil
			}

			directory = &fusecom.DirectoryPlus{directory, statFunc}
		}

		handle, err := fs.directories.Add(directory)
		if err != nil { // TODO: transform error
			fs.log.Error(fuselib.Error(-fuselib.EMFILE))
			return -fuselib.EMFILE, fusecom.ErrorHandle
		}

		return fusecom.OperationSuccess, handle
	}

	// sub requests

	corePath := fs.joinRoot(path)

	var err error
	directory, err = ipfscore.OpenDir(fs.Ctx(), corePath, fs.Core())
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}

	if provcom.CanReaddirPlus {
		stat := new(fuselib.Stat_t)
		var ids fusecom.StatIDGroup
		ids.Uid, ids.Gid, _ = fuselib.Getcontext()
		fusecom.ApplyCommonsToStat(stat, filesWritable, fs.mountTimeGroup, ids)

		statFunc := func(name string) *fuselib.Stat_t {
			switch name {
			case ".":
				if err := fs.coreGetattr(corePath, stat); err != nil {
					return nil // this will force a fallback to full Getattr (which will likely fail again, but will be logged under that call)
				}
			case "..":
				return nil
			default:
				if err := fs.coreGetattr(corepath.Join(corePath, name), stat); err != nil {
					return nil
				}
			}
			return stat
		}

		directory = &fusecom.DirectoryPlus{directory, statFunc}
	}

	handle, err := fs.directories.Add(directory)
	if err != nil { // TODO: transform error
		fs.log.Error(fuselib.Error(-fuselib.EMFILE))
		return -fuselib.EMFILE, fusecom.ErrorHandle
	}

	return fusecom.OperationSuccess, handle
}

func (fs *FileSystem) Releasedir(path string, fh uint64) int {
	fs.log.Debugf("Releasedir - {%X}%q", fh, path)

	goErr, errNo := fusecom.ReleaseDir(fs.directories, fh)
	if goErr != nil {
		fs.log.Error(goErr)
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

	goErr, errNo := fusecom.FillDir(fs.Ctx(), directory, fill, ofst)
	if goErr != nil {
		fs.log.Error(goErr)
	}

	return errNo
}

func (fs *FileSystem) Open(path string, flags int) (int, uint64) {
	fs.Lock()
	defer fs.Unlock()
	fs.log.Debugf("Open - {%X}%q", flags, path)

	goErr, errNo := fusecom.CheckOpenFlagsBasic(filesWritable, flags)
	if goErr != nil {
		fs.log.Error(goErr)
		return errNo, fusecom.ErrorHandle
	}

	goErr, errNo = fusecom.CheckOpenPathBasic(path)
	if goErr != nil {
		fs.log.Error(goErr)
		return errNo, fusecom.ErrorHandle
	}

	corePath := fs.joinRoot(path)

	// TODO: rename back to err when everything else is abstracted
	file, tErr := ipfscore.OpenFile(fs.Ctx(), corePath, fs.Core(), transform.IOFlagsFromFuse(flags))
	if tErr != nil {
		fs.log.Error(tErr)
		return tErr.ToFuse(), fusecom.ErrorHandle
	}

	handle, err := fs.files.Add(file)
	if err != nil { // TODO: transform error
		fs.log.Error(fuselib.Error(-fuselib.EMFILE))
		return -fuselib.EMFILE, fusecom.ErrorHandle
	}

	return fusecom.OperationSuccess, handle
}

func (fs *FileSystem) Release(path string, fh uint64) int {
	fs.log.Debugf("Release - {%X}%q", fh, path)

	goErr, errNo := fusecom.ReleaseFile(fs.files, fh)
	if goErr != nil {
		fs.log.Error(goErr)
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

	err, retVal := fusecom.ReadFile(file, buff, ofst)
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

	// TODO: timeout contexts
	corePath := fs.joinRoot(path)
	linkString, err := ipfscore.Readlink(fs.Ctx(), corePath, fs.Core())
	if err != nil {
		fs.log.Error(err)
		return err.ToFuse(), ""
	}

	// NOTE: paths returned here get sent back to the FUSE library
	// they should not be native paths, regardless of their source format
	return fusecom.OperationSuccess, filepath.ToSlash(linkString)
}
