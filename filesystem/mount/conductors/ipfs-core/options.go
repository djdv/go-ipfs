package ipfsconductor

import (
	gomfs "github.com/ipfs/go-mfs"
)

type (
	Option interface{ apply(*settings) }

	foregroundOpt bool
	mfsOpt        gomfs.Root
)
type settings struct {
	foreground   bool        // should the provider block in the foreground until it exits or run in a background routine
	filesAPIRoot *gomfs.Root // required when mounting the FilesAPI namespace, otherwise nil-able
}

func parseOptions(opts ...Option) *settings {
	settings := new(settings)

	for _, opt := range opts {
		opt.apply(settings)
	}

	return settings
}

// WithFilesRoot provides an MFS root node to use for the FilesAPI namespace
func WithFilesAPIRoot(mroot gomfs.Root) Option {
	return mfsOpt(mroot)
}
func (r mfsOpt) apply(opts *settings) {
	opts.filesAPIRoot = (*gomfs.Root)(&r)
}

// InForeground tells Graft() to block until the provider system returns
func InForeground(b bool) Option {
	return foregroundOpt(b)
}
func (b foregroundOpt) apply(opts *settings) {
	opts.foreground = bool(b)
}
