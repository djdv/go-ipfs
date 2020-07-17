//+build !nofuse

package fuse

import (
	"path/filepath"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	config "github.com/ipfs/go-ipfs-config"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/fuse/sys"
	"github.com/ipfs/go-ipfs/filesystem"
	logging "github.com/ipfs/go-log"
)

type nodeBinding struct {
	nodeInterface filesystem.Interface // interface between FUSE and the target API

	log logging.EventLogger // general operations log

	readdirplusGen      // if set, we'll use this function to equip directories with a means to stat their elements
	filesWritable  bool // switch for metadata fields and operation availability

	files       fileTable // tables for open references
	directories directoryTable

	mountTimeGroup statTimeGroup // artificial file time signatures
}

func (fs *nodeBinding) Init() {
	fs.log.Debug("init")
	/*
		defer func() {
			if fs.initSignal != nil {
				close(fs.initSignal)
			}
			fs.log.Debugf("init finished")
		}()
	*/

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

func (fs *nodeBinding) Destroy() {
	fs.log.Debugf("Destroy - Requested")
}

func (fs *nodeBinding) Statfs(path string, stat *fuselib.Statfs_t) int {
	fs.log.Debugf("Statfs - HostRequest %q", path)

	target, err := config.DataStorePath("")
	if err != nil {
		fs.log.Errorf("Statfs - Config err %q: %v", path, err)
		return -fuselib.ENOENT
	}

	errNo, err := sys.Statfs(target, stat)
	if err != nil {
		fs.log.Errorf("Statfs - err %q: %v", target, err)
	}
	return errNo
}

func (fs *nodeBinding) Readlink(path string) (int, string) {
	fs.log.Debugf("Readlink - %q", path)

	switch path {
	case "/":
		fs.log.Warnf("Readlink - root path is an invalid Request")
		return -fuselib.EINVAL, ""

	case "":
		fs.log.Error("Readlink - empty Request")
		return -fuselib.ENOENT, ""
	}

	linkString, err := fs.nodeInterface.ExtractLink(path)
	if err != nil {
		fs.log.Error(err)
		return interpretError(err), ""
	}

	// NOTE: paths returned here get sent back to the FUSE library
	// they should not be native paths, regardless of their source format
	return operationSuccess, filepath.ToSlash(linkString)
}

func (fs *nodeBinding) Rename(oldpath, newpath string) int {
	fs.log.Warnf("Rename - HostRequest %q->%q", oldpath, newpath)

	if err := fs.nodeInterface.Rename(oldpath, newpath); err != nil {
		fs.log.Error(err)
		return interpretError(err)
	}

	return operationSuccess
}

func (fs *nodeBinding) Truncate(path string, size int64, fh uint64) int {
	fs.log.Debugf("Truncate - HostRequest {%X|%d}%q", fh, size, path)

	if size < 0 {
		return -fuselib.EINVAL
	}

	var didOpen bool
	file, err := fs.files.Get(fh) // use the handle if it's valid
	if err != nil {               // otherwise fallback to open
		file, err = fs.nodeInterface.Open(path, filesystem.IOWriteOnly)
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
