package common

import (
	logging "github.com/ipfs/go-log"
	"github.com/ipfs/go-mfs"
)

type AttachOptions struct {
	Parent WalkRef             // node directly behind self
	Logger logging.EventLogger // what subsystem you are // TODO: maintainer doesn't like this; phase out in favor of pkg variable
	// TODO: separating these out into embedded but distinct types
	// IPFS-core opts
	//ResourceLock  ResourceLock
	// MFSOpts
	MFSRoot *mfs.Root // required for MFS attachments
}

type AttachOption func(*AttachOptions)

func AttachOps(options ...AttachOption) *AttachOptions {
	ops := new(AttachOptions)
	// caller populates what they care about
	for _, op := range options {
		op(ops)
	}

	// insert defaults for empty fields
	if ops.Logger == nil {
		ops.Logger = logging.Logger("FS")
	}

	return ops
}

// if NOT provided, we assume the file system is to be treated as a root, assigning itself as a parent
func Parent(p WalkRef) AttachOption {
	return func(ops *AttachOptions) {
		ops.Parent = p
	}
}

func Logger(l logging.EventLogger) AttachOption {
	return func(ops *AttachOptions) {
		ops.Logger = l
	}
}

func MFSRoot(mroot *mfs.Root) AttachOption {
	return func(ops *AttachOptions) {
		ops.MFSRoot = mroot
	}
}
