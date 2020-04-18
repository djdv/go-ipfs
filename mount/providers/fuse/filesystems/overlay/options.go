package overlay

import (
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	mountcom "github.com/ipfs/go-ipfs/mount/utils/common"
	gomfs "github.com/ipfs/go-mfs"
)

type options struct {
	parent       fuselib.FileSystemInterface // if provided, will be used when refering to ".." of root
	initSignal   fusecom.InitSignal          // if provided, returns a status from fs.Init()
	filesAPIRoot *gomfs.Root                 // if provied, will be mapped to `/file`
	resourceLock mountcom.ResourceLock       // if provided, will replace the default lock used for operations
}

// WithParent provides a reference to a node that will act as a parent to the systems own root
func WithParent(p fuselib.FileSystemInterface) Option {
	return parentOpt(parentOptContainer{p})
}

// WithMFSRoot provides an MFS root node that will be mapped to `/file`
func WithMFSRoot(mroot gomfs.Root) Option {
	return mfsOpt(mroot)
}

// WithResourceLock substitutes the default path locker used for operations by the fs
func WithResourceLock(rl mountcom.ResourceLock) Option {
	return resourceLockOpt(resourceLockOptContainer{rl})
}

// WithInitSignal provides a channel that will receive an error from within fs.Init()
func WithInitSignal(ic chan error) Option {
	return initSignalOpt(ic)
}

type Option interface{ apply(*options) }

type (
	parentOpt                parentOptContainer
	parentOptContainer       struct{ fuselib.FileSystemInterface }
	initSignalOpt            fusecom.InitSignal
	mfsOpt                   gomfs.Root
	resourceLockOpt          resourceLockOptContainer
	resourceLockOptContainer struct{ mountcom.ResourceLock }
)

func (pc parentOpt) apply(opts *options) {
	opts.parent = fuselib.FileSystemInterface(pc.FileSystemInterface)
}

func (r mfsOpt) apply(opts *options) {
	opts.filesAPIRoot = (*gomfs.Root)(&r)
}

func (rc resourceLockOpt) apply(opts *options) {
	opts.resourceLock = mountcom.ResourceLock(rc.ResourceLock)
}

func (ic initSignalOpt) apply(opts *options) {
	opts.initSignal = fusecom.InitSignal(ic)
}
