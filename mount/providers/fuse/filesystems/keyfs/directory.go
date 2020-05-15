package keyfs

import (
	"context"
	"runtime"
	"sync"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	fmfs "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/mfs"
	tmfs "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var _ fuselib.FileSystemInterface = (*rootWrapper)(nil)

// TODO: [review] there's a lot of indirection going on here, some of which might not be necessary
// if we can get away with less, do so, but we also might not be able to avoid it

// TODO: [review] comments are written in AM PD English; make another pass

// TODO: all of these names are nonsense; figure something out
// the structure generates or returns a fuse MFS root
// internally it stores them in a map of keys -> fs

func newRootTable(ctx context.Context, core coreiface.CoreAPI) *mfsWrapper {
	return &mfsWrapper{
		ctx:      ctx,
		core:     core,
		mfsTable: make(mfsTable)}
}

// TODO: override mfs.Open, OpenDir, release, Releasedir
// inc+dec refcount
// finalizer on the fs checks if refcount == 0 and calls destroy

type (
	mfsTable map[string]*mfsRef

	// responsible for assigning an underlying mfs to a key by its name
	// shared across operations on this key
	mfsWrapper struct {
		sync.Mutex // guard table access
		mfsTable   mfsTable

		ctx  context.Context // should be valid for as long as roots+children intend to be accessed via this table or its returned structure
		core coreiface.CoreAPI
	}
)

// multiple file descriptors under the same key will share the same mfs
// so that they may stay in sync with eachother
type mfsRef struct {
	fuselib.FileSystemInterface
	refCount uint
}

func (mr *mfsRef) Open(path string, flags int) (int, uint64) {
	mr.refCount++
	return mr.FileSystemInterface.Open(path, flags)
}
func (mr *mfsRef) Opendir(path string) (int, uint64) {
	mr.refCount++
	return mr.FileSystemInterface.Opendir(path)
}
func (mr *mfsRef) Release(path string, fh uint64) int {
	mr.refCount--
	return mr.FileSystemInterface.Release(path, fh)
}
func (mr *mfsRef) Releasedir(path string, fh uint64) int {
	mr.refCount--
	return mr.FileSystemInterface.Releasedir(path, fh)
}

type rootWrapper struct {
	*mfsRef
}

// TODO: rename; this does open a root but also returns an existing one if we have it
func (mw *mfsWrapper) OpenRoot(key coreiface.Key) (fuselib.FileSystemInterface, error) {
	mw.Lock()
	defer mw.Unlock()

	keyName := key.Name()

	// if we already have an instance of this, use it
	if mfsRef, ok := mw.mfsTable[keyName]; ok {
		mfsRef.refCount++
		rw := &rootWrapper{mfsRef}

		runtime.SetFinalizer(rw, func(rootRef *rootWrapper) {
			mw.Lock()
			defer mw.Unlock()

			rootRef.refCount--
			if rootRef.refCount == 0 {
				delete(mw.mfsTable, keyName)
				rootRef.FileSystemInterface.Destroy()
			}
		})

		return rw, nil
	}

	// otherwise instantiate it
	mroot, err := tmfs.PathToMFSRoot(mw.ctx, key.Path(), mw.core,
		tmfs.IPNSPublisher(keyName, mw.core.Name()))
	if err != nil {
		return nil, err
	}
	fuseMFS := fmfs.NewFileSystem(mw.ctx, *mroot, mw.core)
	// TODO: error check init via channel
	fuseMFS.Init()

	mfsRef := &mfsRef{FileSystemInterface: fuseMFS, refCount: 1}
	mw.mfsTable[keyName] = mfsRef

	// unique object that will fall out of scope
	// triggering the finalizer
	rw := &rootWrapper{mfsRef}

	runtime.SetFinalizer(rw, func(rootRef *rootWrapper) {
		mw.Lock()
		defer mw.Unlock()

		rootRef.refCount--
		if rootRef.refCount == 0 {
			delete(mw.mfsTable, keyName)
			rootRef.FileSystemInterface.Destroy()
		}
	})

	return rw, nil
}
