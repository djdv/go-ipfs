package ipfscore

import (
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	logging "github.com/ipfs/go-log"
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

	var comOpts []fusecom.Option
	for _, opt := range opts {
		if comOpt, isCommon := opt.(commonOpts); isCommon {
			// intercept to apply later
			comOpts = comOpt
			continue
		}
		opt.apply(settings)
	}

	if settings.namespace == mountinter.NamespaceNone {
		settings.namespace = mountinter.NamespaceCore
	}

	// if a log was not provided, provide a more specific default
	comOpts = maybeAppendLog(settings.namespace, comOpts)

	// apply common opts for embedded settings
	fusecom.ApplyOptions(&settings.Settings, comOpts...)

	return settings
}

// XXX: kind of ridiculous but it's nicer on the callers end
func maybeAppendLog(ns mountinter.Namespace, comOpts commonOpts) commonOpts {
	var logWasProvided bool
	for _, opt := range comOpts {
		if _, logWasProvided = opt.(fusecom.LogOpt); logWasProvided {
			break
		}
	}

	if !logWasProvided {
		var logger logging.EventLogger
		switch ns {
		case mountinter.NamespaceIPFS:
			logger = logging.Logger("fuse/ipfs")
		case mountinter.NamespaceIPNS:
			logger = logging.Logger("fuse/ipns")
		default:
			logger = logging.Logger("fuse/ipld")
		}
		comOpts = append(comOpts, fusecom.WithLog(logger))
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

func WithNamespace(ns mountinter.Namespace) Option {
	return namespaceOpt(ns)
}
func (no namespaceOpt) apply(set *settings) {
	set.namespace = mountinter.Namespace(no)
}
