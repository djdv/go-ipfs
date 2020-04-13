package ipns

import (
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	fusemeta "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/meta"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("fuse/ipns")

const fuseSuccess = 0

type Filesystem struct {
	fusemeta.FUSEBase

	fuselib.FileSystemBase // TODO: remove this; should implement everything
}

func (fs *Filesystem) Init() {
	fs.Lock()
	defer fs.Unlock()
	log.Debug("init")

	/*
		fs.handles = make(fsHandles)
		fs.mountTime = fuselib.Now()
	*/

	defer log.Debug("init finished")
	fs.InitSignal <- nil
}

/*
func (fs *Filesystem) Getattr(path string, fStat *fuselib.Stat_t, fh uint64) int {
	fs.Lock()
	defer fs.Unlock()

	log.Debugf("Getattr - Request [%X]%q", fh, path)

	fStat.Uid, fStat.Gid, _ = fuselib.Getcontext()

	if len(path) == 0 { // root entry
		fStat.Mode = fuselib.S_IFDIR | 0755 //TODO: replace all this; hacks (use perm IRXA)
		return fuseSuccess
	}

	// all other entries are looked up
	return -fuselib.ENOENT
	// TODO:
	/*
		get call context
		stat, err := statIPFS(callCtx, path, whatever)
		*fStat = *stat
		return
*/
//}

func (fs *Filesystem) Open(path string, flags int) (int, uint64) {
	fs.Lock()
	defer fs.Unlock()

	switch path {
	case "/" + filename:
		return fuseSuccess, 0
	default:
		return -fuselib.ENOENT, ^uint64(0)
	}

}

func (fs *Filesystem) Releasedir(path string, fh uint64) int {
	return fuseSuccess
}

func (fs *Filesystem) Release(path string, fh uint64) int {
	return fuseSuccess
}

const (
	filename = "hello"
	contents = "hello, world\n"
)

func (self *Filesystem) Getattr(path string, stat *fuselib.Stat_t, fh uint64) (errc int) {
	switch path {
	case "/":
		stat.Mode = fuselib.S_IFDIR | 0555
		return 0
	case "/" + filename:
		stat.Mode = fuselib.S_IFREG | 0444
		stat.Size = int64(len(contents))
		return 0
	default:
		return -fuselib.ENOENT
	}
}

func (self *Filesystem) Read(path string, buff []byte, ofst int64, fh uint64) (n int) {
	endofst := ofst + int64(len(buff))
	if endofst > int64(len(contents)) {
		endofst = int64(len(contents))
	}
	if endofst < ofst {
		return 0
	}
	n = copy(buff, contents[ofst:endofst])
	return
}

func (self *Filesystem) Readdir(path string,
	fill func(name string, stat *fuselib.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) (errc int) {
	fill(".", nil, 0)
	fill("..", nil, 0)
	fill(filename, nil, 0)
	return 0
}
