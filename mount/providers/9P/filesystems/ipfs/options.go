package ipfs

import (
	fuselib "github.com/billziss-gh/cgofuse/fuse"
)

type options struct {
	parent fuselib.FileSystemInterface // if provided, will be used when refering to ".." of root
}

// WithParent provides a reference to a node that will act as a parent to the systems own root
func WithParent(p fuselib.FileSystemInterface) Option {
	return parentOpt(parentOptContainer{p})
}

type Option interface{ apply(*options) }

type (
	parentOpt          parentOptContainer
	parentOptContainer struct{ fuselib.FileSystemInterface }
)

func (pc parentOpt) apply(opts *options) {
	opts.parent = fuselib.FileSystemInterface(pc.FileSystemInterface)
}