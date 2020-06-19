package fuse

import (
	"path/filepath"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	config "github.com/ipfs/go-ipfs-config"
	transform "github.com/ipfs/go-ipfs/filesystem"
	logging "github.com/ipfs/go-log"
)

type fileSystem struct {
	intf transform.Interface // interface between FUSE and IPFS APIs

	initChan InitSignal          // optional message channel to communicate with the caller
	log      logging.EventLogger // general operations log

	readdirplusGen      // if set, we'll use this function to equip directories with a means to stat their elements
	filesWritable  bool // switch for metadata fields and operation availability

	files          fileTable // reference tables
	directories    directoryTable
	mountTimeGroup statTimeGroup // artificial file time signatures
}

func (fs *fileSystem) Init() {
	fs.log.Debug("init")
	defer func() {
		if fs.initChan != nil {
			close(fs.initChan)
		}
		fs.log.Debugf("init finished")
	}()

	fs.files = newFileTable()
	fs.directories = newDirectoryTable()

	timeOfMount := fuselib.Now()

	fs.mountTimeGroup = statTimeGroup{
		atim:     timeOfMount,
		mtim:     timeOfMount,
		ctim:     timeOfMount,
		birthtim: timeOfMount,
	}
}

func (fs *fileSystem) Destroy() {
	fs.log.Debugf("Destroy - Requested")
}

func (fs *fileSystem) Statfs(path string, stat *fuselib.Statfs_t) int {
	fs.log.Debugf("Statfs - Request %q", path)

	target, err := config.DataStorePath("")
	if err != nil {
		fs.log.Errorf("Statfs - Config err %q: %v", path, err)
		return -fuselib.ENOENT
	}

	errNo, err := statfs(target, stat)
	if err != nil {
		fs.log.Errorf("Statfs - err %q: %v", target, err)
	}
	return errNo
}

func (fs *fileSystem) Readlink(path string) (int, string) {
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
		return interpretError(err), ""
	}

	// NOTE: paths returned here get sent back to the FUSE library
	// they should not be native paths, regardless of their source format
	return operationSuccess, filepath.ToSlash(linkString)
}

func (fs *fileSystem) Rename(oldpath string, newpath string) int {
	fs.log.Warnf("Rename - Request %q->%q", oldpath, newpath)

	if err := fs.intf.Rename(oldpath, newpath); err != nil {
		fs.log.Error(err)
		return interpretError(err)
	}

	return operationSuccess
}

func (fs *fileSystem) Truncate(path string, size int64, fh uint64) int {
	fs.log.Debugf("Truncate - Request {%X|%d}%q", fh, size, path)

	if size < 0 {
		return -fuselib.EINVAL
	}

	var didOpen bool
	file, err := fs.files.Get(fh) // use the handle if it's valid
	if err != nil {               // otherwise fallback to open
		file, err = fs.intf.Open(path, transform.IOWriteOnly)
		if err != nil {
			fs.log.Error(err)
			return interpretError(err)
		}
		didOpen = true
	}

	if err = file.Truncate(uint64(size)); err != nil {
		fs.log.Error(err)
		return interpretError(err)
	}

	if didOpen {
		if err := file.Close(); err != nil {
			fs.log.Error(err)
			return interpretError(err)
		}
	}

	return operationSuccess
}
