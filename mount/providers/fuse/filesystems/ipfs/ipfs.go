package ipfs

import (
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	common "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	logging "github.com/ipfs/go-log"
)

var log = logging.Logger("fuse/ipfs")

const fuseSuccess = 0

type Filesystem struct {
	common.FUSEBase

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
	/*
		log.Debugf("Open - Request {%X}%q", flags, path)
		return 0, 0

			if fs.AvailableHandles() == 0 {
				log.Error("Open - all handle slots are filled")
				return -fuse.ENFILE, invalidIndex
			}

			// POSIX specifications
			if flags&O_NOFOLLOW != 0 {
				if indexErr == nil {
					if nodeStat.Mode&fuse.S_IFMT == fuse.S_IFLNK {
						log.Errorf("Open - nofollow requested but %q is a link", path)
						return -fuse.ELOOP, invalidIndex
					}
				}
			}

			if flags&fuse.O_CREAT != 0 {
				switch indexErr {
				case os.ErrNotExist:
					nodeType := unixfs.TFile
					if flags&O_DIRECTORY != 0 {
						nodeType = unixfs.TDirectory
					}

					callContext, cancel := deriveCallContext(fs.ctx)
					defer cancel()
					fErr, gErr := fsNode.Create(callContext, nodeType)
					if gErr != nil {
						log.Errorf("Create - %q: %s", path, gErr)
						return fErr, invalidIndex
					}
					// node was created API side, clear create bits, jump back, and open it FS side
					// respecting link restrictions
					flags &^= (fuse.O_EXCL | fuse.O_CREAT)
					goto lookup

				case nil:
					if flags&fuse.O_EXCL != 0 {
						log.Errorf("Create - exclusive flag provided but %q already exists", path)
						return -fuse.EEXIST, invalidIndex
					}

					if nodeStat.Mode&fuse.S_IFMT == fuse.S_IFDIR {
						if flags&O_DIRECTORY == 0 {
							log.Error("Create - regular file requested but %q resolved to an existing directory", path)
							return -fuse.EISDIR, invalidIndex
						}
					}
				default:
					log.Errorf("Create - unexpected %q: %s", path, indexErr)
					return -fuse.EACCES, invalidIndex
				}
			}

			// Open proper -- resolves reference nodes
			fsNode, err := fs.LookupPath(path)
			if err != nil {
				log.Errorf("Open - path err: %s", err)
				return -fuse.ENOENT, invalidIndex
			}
			fsNode.Lock()
			defer fsNode.Unlock()

			nodeStat, err = fsNode.Stat()
			if err != nil {
				log.Errorf("Open - node %q not initialized", path)
				return -fuse.EACCES, invalidIndex
			}

			if nodeStat.Mode&fuse.S_IFMT != fuse.S_IFLNK {
				return -fuse.ELOOP, invalidIndex //NOTE: this should never happen, lookup should resolve all
			}

			// if request is dir but node is dir
			if (flags&O_DIRECTORY != 0) && (nodeStat.Mode&fuse.S_IFMT != fuse.S_IFDIR) {
				log.Error("Open - Directory requested but %q does not resolve to a directory", path)
				return -fuse.ENOTDIR, invalidIndex
			}

			// if request was file but node is dir
			if (flags&O_DIRECTORY == 0) && (nodeStat.Mode&fuse.S_IFMT == fuse.S_IFDIR) {
				log.Error("Open - regular file requested but %q resolved to a directory", path)
				return -fuse.EISDIR, invalidIndex
			}

			callContext, cancel := deriveCallContext(fs.ctx)
			defer cancel()

			// io is an interface that points to a struct (generic/void*)
			io, err := fsNode.YieldIo(callContext, unixfs.TFile)
			if err != nil {
				log.Errorf("Open - IO err %q %s", path, err)
				return -fuse.EIO, invalidIndex
			}

			// the address of io itself must remain the same across calls
			// as we are sharing it with the OS
			// we use the interface structure itself so that
			// on our side we can change data sources
			// without invalidating handles on the OS/client side
			ifPtr := &io                                     // void *ifPtr = (FsFile*) io;
			handle := uint64(uintptr(unsafe.Pointer(ifPtr))) // uint64_t handle = &ifPtr;
			fsNode.Handles()[handle] = ifPtr                 //GC prevention of our double pointer; free upon Release()

			log.Debugf("Open - Assigned [%X]%q", handle, fsNode)
			return fuseSuccess, handle
	*/
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
