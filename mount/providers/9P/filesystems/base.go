package common

import (
	"context"
	gopath "path"

	ninelib "github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	transcom "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems"
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
	Core          transcom.CoreExtender
}

func NewCoreBase(coreNamespace string, core coreiface.CoreAPI, ops ...AttachOption) CoreBase {
	return CoreBase{
		Base:          NewBase(ops...),
		Core:          &transcom.CoreExtended{core},
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

//TODO: move this elsewhere
// also it needs to be refactored later with the rest of everything
func Readdir(callCtx context.Context, core coreiface.CoreAPI, self corepath.Path, dir transform.Directory, offset uint64) (ninelib.Dirents, error) {
	nineEnts := make(ninelib.Dirents, 0)
	for ent := range dir.List(callCtx, offset) {
		entName := ent.Name()
		entPath, err := core.ResolvePath(callCtx, corepath.Join(self, entName))
		if err != nil {
			return nineEnts, err
		}

		// HACK: quick fix, needs refactor
		iStat, _, err := (&transcom.CoreExtended{core}).Stat(callCtx, entPath, transform.IPFSStatRequestAll)
		if err != nil {
			return nineEnts, err
		}

		nineEnts = append(nineEnts, ninelib.Dirent{
			Name:   entName,
			Offset: ent.Offset(),
			QID:    transform.CidToQID(entPath.Cid(), iStat.FileType),
		})
	}

	return nineEnts, nil
}

// TODO: move this
func IOFlagsFrom9P(nineFlagsAmusementPark ninelib.OpenFlags) transform.IOFlags {
	switch nineFlagsAmusementPark {
	case ninelib.ReadOnly:
		return transform.IOReadOnly
	case ninelib.WriteOnly:
		return transform.IOWriteOnly
	case ninelib.ReadWrite:
		return transform.IOReadWrite
	default:
		return transform.IOFlags(0)
	}
}
