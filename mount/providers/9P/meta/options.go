package meta

import (
	logging "github.com/ipfs/go-log"
	"github.com/ipfs/go-mfs"
)

// TODO: we should separate options out of the meta package

type AttachOptions struct {
	Parent  WalkRef             // node directly behind self
	Logger  logging.EventLogger // what subsystem you are
	MFSRoot *mfs.Root           // required for MFS attachments
}

type AttachOption func(*AttachOptions)

func AttachOps(options ...AttachOption) *AttachOptions {
	ops := &AttachOptions{
		Logger: logging.Logger("FS"),
	}
	for _, op := range options {
		op(ops)
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
