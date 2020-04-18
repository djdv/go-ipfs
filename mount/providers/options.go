package providercommon

import (
	mountcom "github.com/ipfs/go-ipfs/mount/utils/common"
	gomfs "github.com/ipfs/go-mfs"
)

type Options struct {
	ResourceLock mountcom.ResourceLock // if provided, will replace the default lock used for operations
	FilesAPIRoot *gomfs.Root           // required when mounting the FilesAPI namespace, otherwise nil-able
}

// WithFilesRoot provides an MFS root node to use for the FilesAPI namespace
func WithFilesAPIRoot(mroot gomfs.Root) Option {
	return mfsOpt(mroot)
}

// WithResourceLock substitutes the default path locker used for operations by the fs
func WithResourceLock(rl mountcom.ResourceLock) Option {
	return resourceLockOpt(resourceLockOptContainer{rl})
}

type Option interface{ Apply(*Options) }

type (
	resourceLockOpt          resourceLockOptContainer
	resourceLockOptContainer struct{ mountcom.ResourceLock }
	mfsOpt                   gomfs.Root
)

func (rc resourceLockOpt) Apply(opts *Options) {
	opts.ResourceLock = mountcom.ResourceLock(rc.ResourceLock)
}

func (r mfsOpt) Apply(opts *Options) {
	opts.FilesAPIRoot = (*gomfs.Root)(&r)
}
