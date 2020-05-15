// Package overlay dispatches requests to other file system implementations while acting as a single (virtual) file system itself
package overlay

import (
	"context"
	"fmt"
	"runtime"

	"github.com/hugelgupf/p9/fsimpl/templatefs"
	"github.com/hugelgupf/p9/p9"
	common "github.com/ipfs/go-ipfs/mount/providers/9P/filesystems"
	"github.com/ipfs/go-ipfs/mount/providers/9P/filesystems/keyfs"
	"github.com/ipfs/go-ipfs/mount/providers/9P/filesystems/mfs"
	"github.com/ipfs/go-ipfs/mount/providers/9P/filesystems/pinfs"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var _ p9.File = (*File)(nil)
var _ common.WalkRef = (*File)(nil)

/* The overlay File is a file system of itself which wraps other `p9.File` implementations.

It does so by linking the subsystems together in a slash delimited hierarch
and using their methods in the same way a client program would if used independently.

The current mapping looks like this:
(Table key: path - file system its bound to - purpose)
 - /          - returns itself - Maintains the hierarchy and dispatches requests
 - /ipfs      - PinFS - Exposes the node's pins as a series of files
 - /ipfs/*    - IPFS  - Relays requests to the IPFS namespace, translating UnixFS objects into 9P constructs
 - /ipns/     - KeyFS - Exposes the node's keys as a series of files
 - /ipns/*    - IPNS|MFS  - Another relay, but for the IPNS namespace
 - /file     - FilesAPI/MFS - exposes the same root as the node's `ipfs files` storage
*/
type File struct {
	templatefs.NoopFile
	p9.DefaultWalkGetAttr

	common.Base
	common.OverlayBase

	path       uint64
	parent     common.WalkRef
	subsystems map[string]systemTuple
	open       bool
}

// pair a filesystem implementation with directory entry metadata about it
type systemTuple struct {
	file   common.WalkRef
	dirent p9.Dirent
}

// Attacher constructs the default RootIndex file system, and all of its dependants, providing a means to Attach() to it
func Attacher(ctx context.Context, core coreiface.CoreAPI, ops ...common.AttachOption) p9.Attacher {
	options := common.AttachOps(ops...)
	// construct root node actual
	ri := &File{
		Base: common.NewBase(ops...),
		OverlayBase: common.OverlayBase{
			ParentCtx: ctx,
			Opened:    new(uintptr),
		},
		parent: options.Parent,
		path:   common.CidToQIDPath(common.RootPath("/").Cid()),
	}

	// attach to subsystems
	// used for proxying walk requests to other file systems
	type subattacher func(context.Context, coreiface.CoreAPI, ...common.AttachOption) p9.Attacher
	type attachTuple struct {
		string
		subattacher
		logging.EventLogger
	}

	// 9P "Access names" mapped to attacher functions
	subsystems := [...]attachTuple{
		{"ipfs", pinfs.Attacher, logging.Logger("PinFS")},
		{"ipns", keyfs.Attacher, logging.Logger("KeyFS")},
	}

	// allocate root entry pairs
	// assign inherent options,
	// and instantiate a template root entry
	ri.subsystems = make(map[string]systemTuple, len(subsystems))
	subOpts := []common.AttachOption{common.Parent(ri)}
	rootDirent := p9.Dirent{
		Type: p9.TypeDir,
		QID:  p9.QID{Type: p9.TypeDir},
	}

	// couple the strings to their implementations
	// "aname"=>{filesystem,entry}
	for _, subsystem := range subsystems {
		logOpt := common.Logger(subsystem.EventLogger)
		// the file system implementation
		fs, err := subsystem.subattacher(ctx, core, append(subOpts, logOpt)...).Attach()
		if err != nil {
			panic(err) // hard implementation error
		}

		// create a directory entry for it
		rootDirent.Offset++
		rootDirent.Name = subsystem.string

		rootDirent.QID.Path = common.CidToQIDPath(common.RootPath("/" + subsystem.string).Cid())

		// add the fs+entry to the list of subsystems
		ri.subsystems[subsystem.string] = systemTuple{
			file:   fs.(common.WalkRef),
			dirent: rootDirent,
		}
	}

	// attach to files API if provided
	if options.MFSRoot != nil {
		fOpts := append(subOpts,
			common.Logger(logging.Logger("FilesAPI")),
			common.MFSRoot(options.MFSRoot),
		)

		fs, err := mfs.Attacher(ctx, core, fOpts...).Attach()
		if err != nil {
			panic(err) // hard implementation error
		}

		// add the directory entry for it
		rootDirent.Offset++
		rootDirent.Name = "file"
		rootDirent.QID.Path = common.CidToQIDPath(common.RootPath("/" + "file").Cid())

		// bind the fs+entry inside of the subsystem collection
		// (add it for real)
		ri.subsystems["file"] = systemTuple{
			file:   fs.(common.WalkRef),
			dirent: rootDirent,
		}
	} else {
		ri.Logger.Warningf("FilesAPI root was not provided to us, and thus cannot be exposed")
	}

	// detach from our proxied systems when we fall out of memory
	runtime.SetFinalizer(ri, func(root *File) {
		for _, ss := range root.subsystems {
			ss.file.Close()
		}
	})

	return ri
}

func (ri *File) Attach() (p9.File, error) {
	ri.Logger.Debugf("Attach")
	newFid, err := ri.clone()
	if err != nil {
		return nil, err
	}

	return newFid, nil
}

//TODO: enforce 9P standards
func (ri *File) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	ri.Logger.Debug("Open")

	if ri.IsOpen() {
		return p9.QID{}, 0, common.FileOpen
	}

	qid, err := ri.QID()
	if err != nil {
		return p9.QID{}, 0, err
	}

	//atomic.StoreUintptr(ri.Opened, 1)
	ri.open = true

	return qid, 0, nil
}
func (ri *File) Close() error {
	ri.Logger.Debug("Close")

	if ri.open {
		//atomic.StoreUintptr(ri.Opened, 0)
	}

	ri.Closed = true
	return nil
}

func (ri *File) Readdir(offset uint64, count uint32) (p9.Dirents, error) {
	ri.Logger.Debugf("Readdir {%d}", count)

	subsystemCount := len(ri.subsystems)

	switch {
	case uint64(subsystemCount) == offset:
		return nil, nil
	case uint64(subsystemCount) < offset:
		return nil, fmt.Errorf("offset %d extends beyond directory bound %d", offset, subsystemCount)
	}

	relativeEnd := subsystemCount - int(offset)

	// use the lesser for allocating the slice
	var ents p9.Dirents
	if count < uint32(relativeEnd) {
		ents = make(p9.Dirents, count)
	} else {
		ents = make(p9.Dirents, relativeEnd)
	}

	// use ents from map within request bounds to populate slice slots
	for _, pair := range ri.subsystems {
		if count == 0 {
			break
		}
		if pair.dirent.Offset >= offset && pair.dirent.Offset <= uint64(relativeEnd) {
			ents[pair.dirent.Offset-1] = pair.dirent
			count--
		}
	}

	return ents, nil
}

/* WalkRef relevant */

func (ri *File) Fork() (common.WalkRef, error) {
	// make sure we were actually initialized
	if ri.subsystems == nil {
		return nil, common.FSCtxNotInitialized //TODO: not exactly the right error
	}

	// overlay doesn't have any state to fork (yet)

	return ri.clone()
}

// The RootIndex checks if it has attached to "name"
// derives a node from it, and returns it
func (ri *File) Step(name string) (common.WalkRef, error) {
	// consume fs/access name
	subSys, ok := ri.subsystems[name]
	if !ok {
		ri.Logger.Errorf("%q is not provided by us", name)
		return nil, common.ENOENT
	}

	// return a ready to use derivative of it
	return subSys.file.Fork()
}

func (ri *File) QID() (p9.QID, error) {
	return p9.QID{
		Type: p9.TypeDir,
		Path: ri.path,
	}, nil
}
func (ri *File) Backtrack() (common.WalkRef, error) {
	if ri.parent != nil {
		return ri.parent, nil
	}
	return ri, nil
}

/* base class boilerplate */

func (ri *File) Walk(names []string) ([]p9.QID, p9.File, error) {
	ri.Logger.Debugf("Walk %q: %v", ri.String(), names)
	return common.Walker(ri, names)
}

func (ri *File) StatFS() (p9.FSStat, error) {
	ri.Logger.Debug("StatFS")
	return p9.FSStat{
		BlockSize: common.UFS1BlockSize,
		FSID:      common.DevMemory,
	}, nil
}

func (ri *File) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	ri.Logger.Debug("GetAttr")
	return p9.QID{
			Type: p9.TypeDir,
			Path: common.CidToQIDPath(common.RootPath("/").Cid()),
		},
		p9.AttrMask{
			Mode: true,
		},
		p9.Attr{
			Mode: p9.ModeDirectory | common.IRXA,
		},
		nil
}

func (ri *File) clone() (*File, error) {
	// derive a new reference from this instance and return it
	return &File{
		Base:        ri.Base,
		OverlayBase: ri.OverlayBase.Clone(),
		subsystems:  ri.subsystems, // share the same subsystem reference across all instances
		parent:      ri.parent,
		path:        ri.path,
	}, nil
}
