package fuse

import (
	gomfs "github.com/ipfs/go-mfs"
)

type ProviderOption interface{ apply(*providerSettings) }

type providerSettings struct {
	foreground   bool        // should the provider block in the foreground until it exits or run in a background routine
	filesAPIRoot *gomfs.Root // required when mounting the FilesAPI namespace, otherwise nil-able
}

// WithFilesRoot provides an MFS root node to use for the FilesAPI namespace
func WithFilesAPIRoot(mroot *gomfs.Root) ProviderOption {
	return mfsOpt(mfsOptContainer{mroot})
}

type (
	foregroundOpt   bool
	mfsOpt          mfsOptContainer
	mfsOptContainer struct{ *gomfs.Root }
)

func (b foregroundOpt) apply(opts *providerSettings) {
	opts.foreground = bool(b)
}

func (rc mfsOpt) apply(opts *providerSettings) {
	opts.filesAPIRoot = (*gomfs.Root)(rc.Root)
}
