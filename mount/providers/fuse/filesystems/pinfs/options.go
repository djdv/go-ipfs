package pinfs

import (
	"fmt"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	logging "github.com/ipfs/go-log"
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
	//comOpts = maybeAppendLog(comOpts)

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
		comOpts = append(comOpts, fusecom.WithLog(logging.Logger("fuse/pinfs")))
		fmt.Println(comOpts)
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

// WithProxy provides a reference to a node that will act as a proxy for subrequests of this root
func WithProxy(p fuselib.FileSystemInterface) Option {
	return proxyOpt(fsContainer{p})
}
func (fc proxyOpt) apply(set *settings) {
	set.proxy = fuselib.FileSystemInterface(fc.FileSystemInterface)
}
