package fusecommon

import (
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	mountcom "github.com/ipfs/go-ipfs/mount/utils/common"
	logging "github.com/ipfs/go-log"
)

type InitSignal chan error

type Option interface{ apply(*Settings) }

type Settings struct {
	InitSignal InitSignal                  // if provided, will be used to return a status from fs.Init()
	Log        logging.EventLogger         // if provided, will be used as the logger during operations
	Parent     fuselib.FileSystemInterface // if provided, will be used when referring to ".." of root

	// non-nilable
	ResourceLock mountcom.ResourceLock // if provided, will replace the default lock used for operations
}

// ApplyOptions applies the common options to the common settings structure and sets missing defaults
func ApplyOptions(existingSet *Settings, opts ...Option) {
	for _, opt := range opts {
		opt.apply(existingSet)
	}

	// non-nilable defaults
	if existingSet.ResourceLock == nil {
		existingSet.ResourceLock = mountcom.NewResourceLocker()
	}

	if existingSet.Log == nil {
		existingSet.Log = logging.Logger("fuse")
	}
}

// WithInitSignal provides a channel that will receive a signal from within fs.Init()
func WithInitSignal(ic InitSignal) Option {
	return initSignalOpt(ic)
}

// WithLog replaces the default logger
func WithLog(l logging.EventLogger) Option {
	return LogOpt(logOptContainer{l})
}

// WithParent provides a reference to a node that will act as a parent to the systems own root
func WithParent(p fuselib.FileSystemInterface) Option {
	return parentOpt(parentOptContainer{p})
}

// WithResourceLock substitutes the default path locker used for operations by the fs
func WithResourceLock(rl mountcom.ResourceLock) Option {
	return resourceLockOpt(resourceLockOptContainer{rl})
}

type (
	initSignalOpt            InitSignal
	LogOpt                   logOptContainer
	logOptContainer          struct{ logging.EventLogger }
	parentOpt                parentOptContainer
	parentOptContainer       struct{ fuselib.FileSystemInterface }
	resourceLockOpt          resourceLockOptContainer
	resourceLockOptContainer struct{ mountcom.ResourceLock }
)

func (ic initSignalOpt) apply(opts *Settings) {
	opts.InitSignal = InitSignal(ic)
}

func (lc LogOpt) apply(opts *Settings) {
	opts.Log = logging.EventLogger(lc.EventLogger)
}

func (pc parentOpt) apply(opts *Settings) {
	opts.Parent = fuselib.FileSystemInterface(pc.FileSystemInterface)
}

func (rc resourceLockOpt) apply(opts *Settings) {
	opts.ResourceLock = mountcom.ResourceLock(rc.ResourceLock)
}
