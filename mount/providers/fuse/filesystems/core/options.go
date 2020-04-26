package ipfscore

import (
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
)

type (
	Option interface{ apply(*settings) }

	commonOpts   []fusecom.Option
	namespaceOpt mountinter.Namespace
)

type settings struct {
	fusecom.Settings
	namespace mountinter.Namespace // TODO: document this
	//TODO: operation timeout time.Time
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

func WithNamespace(ns mountinter.Namespace) Option {
	return namespaceOpt(ns)
}
func (no namespaceOpt) apply(set *settings) {
	set.namespace = mountinter.Namespace(no)
}
