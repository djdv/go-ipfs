package ipfs

import (
	"context"
	"io"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	mountcom "github.com/ipfs/go-ipfs/mount/utils/common"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

var log logging.EventLogger = logging.Logger("fuse/ipfs")

const todoHandle = 0

var _ fuselib.FileSystemInterface = (*FileSystem)(nil)

type FileSystem struct {
	provcom.IPFSCore

	initChan fusecom.InitSignal

	file      transform.File
	directory transform.Directory

	// TODO: remove this; packages should implement all interface methods within the package
	fuselib.FileSystemBase
}

func NewFileSystem(ctx context.Context, core coreiface.CoreAPI, opts ...Option) *FileSystem {
	options := new(options)
	for _, opt := range opts {
		opt.apply(options)
	}

	if options.resourceLock == nil {
		options.resourceLock = mountcom.NewResourceLocker()
	}

	if options.log != nil {
		log = options.log
	}

	return &FileSystem{
		IPFSCore: provcom.NewIPFSCore(ctx, core, options.resourceLock),
		initChan: options.initSignal,
	}
}

func (fs *FileSystem) Init() {
	fs.Lock()
	defer fs.Unlock()
	log.Debug("init")

	/*
		fs.handles = make(fsHandles)
		fs.mountTime = fuselib.Now()
	*/

	defer log.Debug("init finished")
	if c := fs.initChan; c != nil {
		c <- nil
	}
}

func (fs *FileSystem) Open(path string, flags int) (int, uint64) {
	fs.Lock()
	defer fs.Unlock()
	log.Debugf("Open - {%X}%q", flags, path)

	switch path {
	case "":
		// TODO: handle empty path (valid if fh is valid)
		log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT, fusecom.ErrorHandle
	case "/":
		log.Error(fuselib.Error(-fuselib.EISDIR))
		return -fuselib.EISDIR, fusecom.ErrorHandle
	default:
		file, err := transform.CoreOpenFile(fs.Ctx(), corepath.New(path[1:]), fs.Core(), transform.IOFlagsFromFuse(flags))
		if err != nil {
			// TODO: proper error translations transError.ToFuse(), etc.
			// EIO might not be appropriate here either ref: POSIX open()
			log.Error(fuselib.Error(-fuselib.EIO))
			return -fuselib.EIO, todoHandle
		}
		fs.file = file
	}

	return fusecom.OperationSuccess, todoHandle

	/* old and gross; translate and slavage what you can from this arcane mess
	log.Debugf("Open - Request {%X}%q", flags, path)
	return 0, 0

		if fs.AvailableHandles() == 0 {
			log.Error("Open - all handle slots are filled")
			return -fuselib.ENFILE, invalidIndex
		}

		// POSIX specifications
		if flags&O_NOFOLLOW != 0 {
			if indexErr == nil {
				if nodeStat.Mode&fuselib.S_IFMT == fuselib.S_IFLNK {
					log.Errorf("Open - nofollow requested but %q is a link", path)
					return -fuselib.ELOOP, invalidIndex
				}
			}
		}

		if flags&fuselib.O_CREAT != 0 {
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
				flags &^= (fuselib.O_EXCL | fuselib.O_CREAT)
				goto lookup

			case nil:
				if flags&fuselib.O_EXCL != 0 {
					log.Errorf("Create - exclusive flag provided but %q already exists", path)
					return -fuselib.EEXIST, invalidIndex
				}

				if nodeStat.Mode&fuselib.S_IFMT == fuselib.S_IFDIR {
					if flags&O_DIRECTORY == 0 {
						log.Error("Create - regular file requested but %q resolved to an existing directory", path)
						return -fuselib.EISDIR, invalidIndex
					}
				}
			default:
				log.Errorf("Create - unexpected %q: %s", path, indexErr)
				return -fuselib.EACCES, invalidIndex
			}
		}

		// Open proper -- resolves reference nodes
		fsNode, err := fs.LookupPath(path)
		if err != nil {
			log.Errorf("Open - path err: %s", err)
			return -fuselib.ENOENT, invalidIndex
		}
		fsNode.Lock()
		defer fsNode.Unlock()

		nodeStat, err = fsNode.Stat()
		if err != nil {
			log.Errorf("Open - node %q not initialized", path)
			return -fuselib.EACCES, invalidIndex
		}

		if nodeStat.Mode&fuselib.S_IFMT != fuselib.S_IFLNK {
			return -fuselib.ELOOP, invalidIndex //NOTE: this should never happen, lookup should resolve all
		}

		// if request is dir but node is dir
		if (flags&O_DIRECTORY != 0) && (nodeStat.Mode&fuselib.S_IFMT != fuselib.S_IFDIR) {
			log.Error("Open - Directory requested but %q does not resolve to a directory", path)
			return -fuselib.ENOTDIR, invalidIndex
		}

		// if request was file but node is dir
		if (flags&O_DIRECTORY == 0) && (nodeStat.Mode&fuselib.S_IFMT == fuselib.S_IFDIR) {
			log.Error("Open - regular file requested but %q resolved to a directory", path)
			return -fuselib.EISDIR, invalidIndex
		}

		callContext, cancel := deriveCallContext(fs.ctx)
		defer cancel()

		// io is an interface that points to a struct (generic/void*)
		io, err := fsNode.YieldIo(callContext, unixfs.TFile)
		if err != nil {
			log.Errorf("Open - IO err %q %s", path, err)
			return -fuselib.EIO, invalidIndex
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
		return fusecom.FuseSuccess, handle
	*/
}

func (fs *FileSystem) Opendir(path string) (int, uint64) {
	log.Debugf("Opendir - %q", path)
	switch path {
	case "":
		log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT, fusecom.ErrorHandle

	case "/":
		// TODO: return valid empty dir
		log.Error(fuselib.Error(-fuselib.EISDIR))
		return -fuselib.EISDIR, fusecom.ErrorHandle

	default:
		directory, err := transform.CoreOpenDir(fs.Ctx(), corepath.New(path[1:]), fs.Core())
		if err != nil {
			log.Error(err)
			return -fuselib.ENOENT, fusecom.ErrorHandle
		}
		fs.directory = directory
		return fusecom.OperationSuccess, todoHandle
	}
}

func (fs *FileSystem) Releasedir(path string, fh uint64) int {
	return fusecom.OperationSuccess
}

func (fs *FileSystem) Release(path string, fh uint64) int {
	return fusecom.OperationSuccess
}

func (fs *FileSystem) Getattr(path string, stat *fuselib.Stat_t, fh uint64) int {
	log.Debugf("Getattr - {%X}%q", fh, path)
	switch path {
	case "/":
		stat.Mode = fuselib.S_IFDIR | 0555
		return fusecom.OperationSuccess
		// TODO: handle empty path (valid if fh is valid)
	default:
		// expectation is to receive `/${multihash}`, not `/ipfs/${mulithash}`
		iStat, _, err := transform.GetAttrCore(fs.Ctx(), corepath.New(path[1:]), fs.Core(), transform.IPFSStatRequestAll)
		if err != nil {
			log.Error(err)
			return -fuselib.ENOENT
		}

		*stat = *iStat.ToFuse()
		fusecom.ApplyPermissions(false, &stat.Mode)
		return fusecom.OperationSuccess
	}
}

func (fs *FileSystem) Read(path string, buff []byte, ofst int64, fh uint64) int {
	log.Debugf("Read - {%X}%q", fh, path)

	// TODO: [review] we need to do things on failure
	// the OS typically triggers a close, but we shouldn't expect it to invalidate this record for us
	// we also might want to store a file cursor to reduce calls to seek
	// the same thing already happens internally so it's at worst the overhead of a call right now

	if ofst < 0 {
		log.Errorf("Read - Invalid offset {%d}[%X]%q", ofst, fh, path)
		return -fuselib.EINVAL
	}

	if fileBound, err := fs.file.Size(); err == nil {
		if ofst >= fileBound {
			return 0 // POSIX expects this
		}
	}

	if ofst != 0 {
		_, err := fs.file.Seek(ofst, io.SeekStart)
		if err != nil {
			log.Errorf("Read - seek error: %s", err)
			return -fuselib.EIO
		}
	}

	readBytes, err := fs.file.Read(buff)
	if err != nil && err != io.EOF {
		log.Errorf("Read - error: %s", err)
		return -fuselib.EIO
	}
	return readBytes
}

func (fs *FileSystem) Readdir(path string,
	fill func(name string, stat *fuselib.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) (errc int) {
	log.Debugf("Readdir - {%X|%d}%q", fh, ofst, path)
	// TODO: handle root path (empty directory)

	// TODO: use fh instead of path for this
	if fs.directory == nil {
		log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	// TODO: [audit] int -> uint needs range checking
	entChan, err := fs.directory.Read(uint64(ofst), 0).ToFuse()
	if err != nil {
		// TODO: inspect error
		log.Error(fuselib.Error(-fuselib.EBADF))
		return -fuselib.EBADF
	}

	fill(".", nil, 0)
	fill("..", nil, 0) // if parent !nil; parent.Getattr().ToFuse()

	for ent := range entChan {
		log.Debugf("ent pre: %#v", ent)
		fusecom.ApplyPermissions(false, &ent.Stat.Mode)
		log.Debugf("ent post: %#v", ent)
		if !fill(ent.Name, ent.Stat, ent.Offset) {
			log.Debugf("Readdir - aborting, fill dropped {[%X]%q}", fh, path)
			break
		}
	}
	return fusecom.OperationSuccess
}
