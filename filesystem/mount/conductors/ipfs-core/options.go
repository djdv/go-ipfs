package ipfsconductor

import (
	"github.com/ipfs/go-ipfs/filesystem/mount"
	logging "github.com/ipfs/go-log"
	gomfs "github.com/ipfs/go-mfs"
)

type (
	Option interface{ apply(*conductorSettings) }

	foregroundOpt bool
	mfsOpt        gomfs.Root
)

type conductorSettings struct {
	foreground   bool        // should the provider block in the foreground until it exits or run in a background routine
	filesAPIRoot *gomfs.Root // required when mounting the FilesAPI namespace, otherwise nil-able
	log          logging.EventLogger
}

func parseConductorOptions(options ...Option) *conductorSettings {
	settings := new(conductorSettings)

	for _, opt := range options {
		opt.apply(settings)
	}

	if settings.log == nil {
		settings.log = logging.Logger(mount.LogGroup + "/conductor")
	}

	return settings
}

// WithFilesRoot provides an MFS root node to use for the FilesAPI namespace
func WithFilesAPIRoot(mroot gomfs.Root) Option {
	return mfsOpt(mroot)
}

func (r mfsOpt) apply(opts *conductorSettings) {
	opts.filesAPIRoot = (*gomfs.Root)(&r)
}

// InForeground tells Graft() to block until the provider system returns
func InForeground(b bool) Option {
	return foregroundOpt(b)
}

func (b foregroundOpt) apply(opts *conductorSettings) {
	opts.foreground = bool(b)
}
