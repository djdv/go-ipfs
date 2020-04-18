package ipfsconductor

import (
	"github.com/ipfs/go-mfs"
)

type options struct {
	foreground   bool      // should the provider block in the foreground until it exits or run in a background routine
	filesAPIRoot *mfs.Root // required when mounting the FilesAPI namespace, otherwise nil-able
}

// WithFilesRoot provides an MFS root node to use for the FilesAPI namespace
func WithFilesAPIRoot(mroot mfs.Root) Option {
	return mfsOpt(mroot)
}

// InForeground tells Graft() to block until the provider system returns
func InForeground(b bool) Option {
	return foregroundOpt(b)
}

type Option interface{ apply(*options) }

type (
	foregroundOpt bool
	mfsOpt        mfs.Root
)

func (b foregroundOpt) apply(opts *options) {
	opts.foreground = bool(b)
}

func (r mfsOpt) apply(opts *options) {
	opts.filesAPIRoot = (*mfs.Root)(&r)
}
