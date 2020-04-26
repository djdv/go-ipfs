package pinfs

import (
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
)

type (
	Option interface{ apply(*settings) }

	commonOpts  []fusecom.Option
	fsContainer struct{ fuselib.FileSystemInterface }
	proxyOpt    fsContainer
)

type settings struct {
	fusecom.Settings
	proxy fuselib.FileSystemInterface // if provided, will be used to relay subdirectory requests to another system
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

// WithProxy provides a reference to a node that will act as a proxy for subrequests of this root
func WithProxy(p fuselib.FileSystemInterface) Option {
	return proxyOpt(fsContainer{p})
}
func (fc proxyOpt) apply(set *settings) {
	set.proxy = fuselib.FileSystemInterface(fc.FileSystemInterface)
}
