package overlay

import (
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	gomfs "github.com/ipfs/go-mfs"
)

type (
	Option interface{ apply(*settings) }

	commonOpts []fusecom.Option
	mfsOpt     gomfs.Root
)

type settings struct {
	fusecom.Settings
	filesAPIRoot *gomfs.Root // if provied, will be mapped to `/file`
}

func parseOptions(opts ...Option) *settings {
	settings := new(settings)
	for _, opt := range opts {
		opt.apply(settings)
	}

	// set defaults for embedded
	fusecom.ApplyOptions(&settings.Settings)

	return settings
}

// WithCommon applies the common options shared by our filesystem implementations
func WithCommon(opts ...fusecom.Option) Option {
	return commonOpts(opts)
}
func (co commonOpts) apply(set *settings) {
	fusecom.ApplyOptions(&set.Settings, ([]fusecom.Option)(co)...)
}

// WithMFSRoot provides an MFS root node that will be mapped to `/file`
func WithMFSRoot(mroot gomfs.Root) Option {
	return mfsOpt(mroot)
}
func (r mfsOpt) apply(set *settings) {
	set.filesAPIRoot = (*gomfs.Root)(&r)
}
