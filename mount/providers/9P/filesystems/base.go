package common

import (
	gopath "path"

	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

// Base provides a foundation to build file system nodes which contain file meta data
// as well as some base methods.
type Base struct {
	Trail  []string // FS "breadcrumb" trail from node's root
	Logger logging.EventLogger
}

func NewBase(ops ...AttachOption) Base {
	options := AttachOps(ops...)
	if options.Logger == nil {
		options.Logger = logging.Logger("FS")
	}

	return Base{Logger: options.Logger}
}

// CoreBase extends the base to hold metadata specific to the IPFS CoreAPI
type CoreBase struct {
	Base

	/*	Format the namespace as if it were a rooted directory, sans trailing slash.
		e.g. `/ipfs`
		The base relative path is appended to the namespace for core requests upon calling `.CorePath()`.
	*/
	CoreNamespace string
	Core          coreiface.CoreAPI
}

func NewCoreBase(coreNamespace string, core coreiface.CoreAPI, ops ...AttachOption) CoreBase {
	return CoreBase{
		Base:          NewBase(ops...),
		Core:          core,
		CoreNamespace: coreNamespace,
	}
}

func (b *Base) String() string { return gopath.Join(b.Trail...) }
func (ib *CoreBase) String() string {
	return gopath.Join(append([]string{ib.CoreNamespace}, ib.Base.String())...)
}
func (ib *CoreBase) CorePath(names ...string) corepath.Path {
	if len(ib.Trail) == 0 && len(names) == 0 {
		return RootPath(ib.CoreNamespace)
	}
	return corepath.Join(RootPath(ib.CoreNamespace), append(ib.Trail, names...)...)
}
