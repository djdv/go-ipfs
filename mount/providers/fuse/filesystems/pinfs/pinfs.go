package pinfs

import (
	"context"
	"io"
	"path/filepath"
	"time"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	pinfs "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/pinfs"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var _ fuselib.FileSystemInterface = (*FileSystem)(nil)

const filesWritable = false

type FileSystem struct {
	fusecom.SharedMethods

	initChan fusecom.InitSignal

	intf transform.Interface

	files       fusecom.FileTable
	directories fusecom.DirectoryTable

	log logging.EventLogger

	mountTimeGroup fusecom.StatTimeGroup
	rootStat       *fuselib.Stat_t
}

func NewFileSystem(ctx context.Context, core coreiface.CoreAPI, opts ...Option) *FileSystem {
	settings := parseOptions(opts...)

	return &FileSystem{
		intf:     pinfs.NewInterface(ctx, core),
		initChan: settings.InitSignal,
		log:      settings.Log,
	}
}

func (fs *FileSystem) Init() {
	fs.log.Debug("init")
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

func (fs *FileSystem) Getattr(path string, stat *fuselib.Stat_t, fh uint64) (errc int) {
	fs.log.Debugf("Getattr - {%X}%q", fh, path)

	if path == "/" { // root request
		*stat = *fs.rootStat
		stat.Uid, stat.Gid, _ = fuselib.Getcontext()
		return fusecom.OperationSuccess
	}

	iStat, _, err := fs.intf.Info(path, transform.IPFSStatRequestAll)
	if err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err)
	}

	var ids fusecom.StatIDGroup
	ids.Uid, ids.Gid, _ = fuselib.Getcontext()
	fusecom.ApplyIntermediateStat(stat, iStat)
	fusecom.ApplyCommonsToStat(stat, filesWritable, fs.mountTimeGroup, ids)
	return fusecom.OperationSuccess

}

func (fs *FileSystem) Opendir(path string) (int, uint64) {
	fs.log.Debugf("Opendir - %q", path)

	directory, err := fs.intf.OpenDirectory(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}
	handle, err := fs.directories.Add(directory)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EMFILE, fusecom.ErrorHandle
	}
	return fusecom.OperationSuccess, handle
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

	file, err := fs.intf.Open(path, fusecom.IOFlagsFromFuse(flags))
	if err != nil {
		fs.log.Error(err)
		return fusecom.InterpretError(err), fusecom.ErrorHandle
	}

	handle, err := fs.files.Add(file)
	if err != nil {
		fs.log.Error(err)
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
	fs.log.Debugf("Read - {%X}%q", fh, path)

	if path == "/" { // root request; we're never a file
		fs.log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	file, err := fs.files.Get(fh)
	if err != nil {
		fs.log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	errNo, err := fusecom.ReadFile(file, buff, ofst)
	if err != nil && err != io.EOF {
		fs.log.Error(err)
	}

	return errNo
}

func (fs *FileSystem) Readlink(path string) (int, string) {
	fs.log.Debugf("Readlink - %q", path)

	if path == "/" {
		return -fuselib.EINVAL, ""
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
