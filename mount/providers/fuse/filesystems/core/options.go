package ipfscore

import (
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	mountcom "github.com/ipfs/go-ipfs/mount/utils/common"
	logging "github.com/ipfs/go-log"
)

type options struct {
	parent       fuselib.FileSystemInterface // if provided, will be used when refering to ".." of root
	initSignal   fusecom.InitSignal          // if provided, returns a status from fs.Init()
	resourceLock mountcom.ResourceLock       // if provided, will replace the default lock used for operations
	log          logging.EventLogger         // TODO: document this
	namespace    mountinter.Namespace        // TODO: document this
}

// WithParent provides a reference to a node that will act as a parent to the systems own root
func WithParent(p fuselib.FileSystemInterface) Option {
	return parentOpt(parentOptContainer{p})
}

// WithInitSignal provides a channel that will receive an error from within fs.Init()
func WithInitSignal(ic chan error) Option {
	return initSignalOpt(ic)
}

// WithResourceLock substitutes the default path locker used for operations by the fs
func WithResourceLock(rl mountcom.ResourceLock) Option {
	return resourceLockOpt(resourceLockOptContainer{rl})
}

// WithLog replaces the default logger
func WithLog(l logging.EventLogger) Option {
	return logOpt(logOptContainer{l})
}

func WithNamespace(ns mountinter.Namespace) Option {
	return namespaceOpt(ns)
}

type Option interface{ apply(*options) }

type (
	parentOpt                parentOptContainer
	parentOptContainer       struct{ fuselib.FileSystemInterface }
	initSignalOpt            fusecom.InitSignal
	resourceLockOpt          resourceLockOptContainer
	resourceLockOptContainer struct{ mountcom.ResourceLock }
	logOpt                   logOptContainer
	logOptContainer          struct{ logging.EventLogger }
	namespaceOpt             mountinter.Namespace
)

func (no namespaceOpt) apply(opts *options) {
	opts.namespace = mountinter.Namespace(no)
}

func (pc parentOpt) apply(opts *options) {
	opts.parent = fuselib.FileSystemInterface(pc.FileSystemInterface)
}

func (ic initSignalOpt) apply(opts *options) {
	opts.initSignal = fusecom.InitSignal(ic)
}

func (rc resourceLockOpt) apply(opts *options) {
	opts.resourceLock = mountcom.ResourceLock(rc.ResourceLock)
}
func (lc logOpt) apply(opts *options) {
	opts.log = logging.EventLogger(lc.EventLogger)
}
