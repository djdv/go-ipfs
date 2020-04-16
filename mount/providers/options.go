package provider

import "github.com/ipfs/go-mfs"

type (
	Option  func(*Options)
	Options struct {
		ProviderParameter string // a provider specific string used during initialization; this should be documented by the provider implementation if required
		//Foreground        bool   // should the provider block in the foreground until it exits or run in a background routine

		FilesRoot *mfs.Root // required when mounting the Files namespace, otherwise nil-able
	}
)

func ProviderFilesRoot(root *mfs.Root) Option {
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
