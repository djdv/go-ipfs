package conductor

import "github.com/ipfs/go-mfs"

// TODO: we should split mfs out of this; let core have its own embedded version

type (
	Option  func(*Options)
	Options struct {
		Foreground bool // should the provider block in the foreground until it exits or run in a background routine

		FilesRoot *mfs.Root // required when mounting the Files namespace, otherwise nil-able
	}
)

func MountForeground(cond bool) Option {
	return func(ops *Options) {
		ops.Foreground = cond
	}
}

func MountFilesRoot(root *mfs.Root) Option {
	return func(ops *Options) {
		ops.FilesRoot = root
	}
}

func ParseOptions(options ...Option) *Options {
	processedOps := &Options{}

	for _, applyOption := range options {
		applyOption(processedOps)
	}
	return processedOps
}
