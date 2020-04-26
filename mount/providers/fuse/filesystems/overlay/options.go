package overlay

import (
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	logging "github.com/ipfs/go-log"
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

	var comOpts []fusecom.Option
	for _, opt := range opts {
		if comOpt, isCommon := opt.(commonOpts); isCommon {
			// intercept to apply later
			comOpts = comOpt
			continue
		}
		opt.apply(settings)
	}

	// if a log was not provided, provide a more specific default
	comOpts = maybeAppendLog(comOpts)

	// apply common opts for embedded settings
	fusecom.ApplyOptions(&settings.Settings, comOpts...)

	return settings
}

// XXX: kind of ridiculous but it's nicer on the callers end
func maybeAppendLog(comOpts commonOpts) commonOpts {
	var logWasProvided bool
	for _, opt := range comOpts {
		if _, logWasProvided = opt.(fusecom.LogOpt); logWasProvided {
			break
		}
	}
	if !logWasProvided {
		comOpts = append(comOpts, fusecom.WithLog(logging.Logger("fuse/overlay")))
	}
	return comOpts
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
