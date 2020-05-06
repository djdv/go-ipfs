package keyfs

import (
	"context"
	"errors"
	"fmt"
	"strings"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	provcom "github.com/ipfs/go-ipfs/mount/providers"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	ipfscore "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/core"
	"github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/mfs"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/keyfs"
	tmfs "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/mfs"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	coreoptions "github.com/ipfs/interface-go-ipfs-core/options"
)

var _ fuselib.FileSystemInterface = (*FileSystem)(nil)

type FileSystem struct {
	fusecom.SharedMethods
	provcom.IPFSCore

	initChan fusecom.InitSignal

	directories fusecom.DirectoryTable

	log  logging.EventLogger
	ipns fuselib.FileSystemInterface

	//TODO: these are going to need open reference counts so they can be deleted on close
	mfsInstances map[string]*mfs.FileSystem
	//TODO: uioInstances map[string]uio.Modifier
}

func NewFileSystem(ctx context.Context, core coreiface.CoreAPI, opts ...Option) *FileSystem {
	settings := parseOptions(opts...)

	return &FileSystem{
		IPFSCore:     provcom.NewIPFSCore(ctx, core, settings.ResourceLock),
		initChan:     settings.InitSignal,
		log:          settings.Log,
		mfsInstances: make(map[string]*mfs.FileSystem),
	}
}

// string: subpath
func (fs *FileSystem) selectFS(path string) (fuselib.FileSystemInterface, string, error) {
	switch path {
	case "":
		return nil, "", errors.New("empty path")
	case "/":
		return fs, "", nil
	default:
		if path[0] != '/' {
			return nil, "", errors.New("invalid path")
		}

		slashIndex := 1 // skip leading slash
		slashIndex += strings.IndexRune(path[1:], '/')

		var namespace, pathRemainder string
		if slashIndex == 0 { // input looks like: `/namespace`
			namespace = path[1:]
			pathRemainder = "/"
		} else { // input looks like: `/namespace/sub...`
			namespace = path[1:slashIndex]
			pathRemainder = path[slashIndex:]
		}

		if namespace == "" {
			return fs, pathRemainder, nil
		}

		keyFS, keyFound, err := fs.checkKey(namespace)
		if keyFound {
			if err != nil {
				return nil, pathRemainder, err
			}
			return keyFS, pathRemainder, nil
		}

		// not an owned key, relay to IPNS resolver
		return fs.ipns, pathRemainder, nil
	}
}

func offlineAPI(core coreiface.CoreAPI) coreiface.CoreAPI {
	oAPI, err := core.WithOptions(coreoptions.Api.Offline(true))
	if err != nil {
		panic(err)
	}
	return oAPI
}

// bool:keyFound?
func (fs *FileSystem) checkKey(keyName string) (fuselib.FileSystemInterface, bool, error) {
	callContext, cancel := context.WithCancel(fs.Ctx())
	defer cancel()

	keys, err := fs.Core().Key().List(callContext)
	if err != nil {
		return nil, false, err
	}

	for _, key := range keys {
		if key.Name() == keyName {
			// NOTE: keys that have not been published to will not resolve
			// their CID points to something, but I'm not sure what even
			// TODO: ask someone about this or check init code
			// hash on this node is set to "QmTTfK7tAW76GE1uFqU6vfLhHor5LhvjjoKQwrcB23Dryv"

			// path points to a key we own, check its type
			iState, _, err := transform.GetAttr(fs.Ctx(), key.Path(), offlineAPI(fs.Core()), transform.IPFSStatRequest{Type: true})
			if err != nil {
				return nil, true, err
			}
			switch ft := iState.FileType; ft {
			case coreiface.TFile:
				//TODO: return self with remainder; handle construction in Open
				return nil, true, errors.New("key's as files not supported yet")

			case coreiface.TDirectory:
				// if we already have an instance of this, use it
				if fs, ok := fs.mfsInstances[keyName]; ok {
					return fs, true, nil
				}
				// otherwise instantiate it
				mroot, err := tmfs.PathToMFSRoot(fs.Ctx(), key.Path(), fs.Core(),
					tmfs.IPNSPublisher(key.Name(), fs.Core().Name()))
				if err != nil {
					return nil, true, err
				}
				mfsKey := mfs.NewFileSystem(fs.Ctx(), *mroot, fs.Core())
				fs.mfsInstances[keyName] = mfsKey
				return mfsKey, true, nil

			default:
				return nil, true, fmt.Errorf("unsupported key type: %v", ft)
			}
		}
	}
	return nil, false, nil
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
	fs.directories = fusecom.NewDirectoryTable()
}

func (fs *FileSystem) Destroy() {
	fs.log.Debugf("Destroy - Requested")
}

func (fs *FileSystem) Getattr(path string, stat *fuselib.Stat_t, fh uint64) (errc int) {
	fs.log.Debugf("Getattr - {%X}%q", fh, path)

	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT
	}

	if targetFs == fs {
		// TODO: writable
		stat.Mode = fuselib.S_IFDIR
		fusecom.ApplyPermissions(false, &stat.Mode)
		stat.Uid, stat.Gid, _ = fuselib.Getcontext()
		return fusecom.OperationSuccess
	}

	return targetFs.Getattr(remainder, stat, fh)
}

func (fs *FileSystem) Opendir(path string) (int, uint64) {
	fs.log.Debugf("Opendir - %q", path)

	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.ENOENT, fusecom.ErrorHandle
	}

	if targetFs == fs {
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
	return targetFs.Opendir(remainder)
}

func (fs *FileSystem) Releasedir(path string, fh uint64) int {
	fs.log.Debugf("Releasedir - {%X}%q", fh, path)

	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EBADF
	}

	if targetFs == fs {
		goErr, errNo := fusecom.ReleaseDir(fs.directories, fh)
		if goErr != nil {
			fs.log.Error(goErr)
		}

		return errNo
	}

	return targetFs.Releasedir(remainder, fh)
}

func (fs *FileSystem) Readdir(path string,
	fill func(name string, stat *fuselib.Stat_t, ofst int64) bool,
	ofst int64,
	fh uint64) int {
	fs.log.Debugf("Readdir - {%X|%d}%q", fh, ofst, path)

	targetFs, remainder, err := fs.selectFS(path)
	if err != nil {
		fs.log.Error(err)
		return -fuselib.EBADF
	}

	if targetFs == fs {
		directory, err := fs.directories.Get(fh)
		if err != nil {
			fs.log.Error(fuselib.Error(-fuselib.EBADF))
			return -fuselib.EBADF
		}

		goErr, errNo := fusecom.FillDir(fs.Ctx(), directory, false, fill, ofst)
		if goErr != nil {
			fs.log.Error(goErr)
		}

		return errNo
	}

	return targetFs.Readdir(remainder, fill, ofst, fh)
}

func (fs *FileSystem) Open(path string, flags int) (int, uint64) {
	fs.log.Debugf("Open - {%X}%q", flags, path)

	switch path {
	case "":
		fs.log.Error(fuselib.Error(-fuselib.ENOENT))
		return -fuselib.ENOENT, fusecom.ErrorHandle

	case "/":
		fs.log.Error(fuselib.Error(-fuselib.EISDIR))
		return -fuselib.EISDIR, fusecom.ErrorHandle

	default:
		return fs.ipns.Open(path, flags)
	}
}

func (fs *FileSystem) Release(path string, fh uint64) int {
	fs.log.Debugf("Release - {%X}%q", fh, path)

	return fs.ipns.Release(path, fh)
}

func (fs *FileSystem) Read(path string, buff []byte, ofst int64, fh uint64) int {
	fs.log.Debugf("Read - {%X}%q", fh, path)

	return fs.ipns.Read(path, buff, ofst, fh)
}

func (fs *FileSystem) Readlink(path string) (int, string) {
	fs.log.Debugf("Readlink - %q", path)

	switch path {
	default:
		return fs.ipns.Readlink(path)

	case "/":
		fs.log.Warnf("Readlink - root path is an invalid request")
		return -fuselib.EINVAL, ""

	case "":
		fs.log.Error("Readlink - empty request")
		return -fuselib.ENOENT, ""
	}
}
