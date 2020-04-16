// Package keyfs acts as an overlay of IPNS and MFS
// dispatching requests to MFS if we have access to its key
// and otherwise defering to IPNS
package keyfs

import (
	"context"
	"errors"
	gopath "path"
	"runtime"
	"sync"
	"time"

	"github.com/hugelgupf/p9/fsimpl/templatefs"
	"github.com/hugelgupf/p9/p9"
	cid "github.com/ipfs/go-cid"
	common "github.com/ipfs/go-ipfs/mount/providers/9P/filesystems"
	"github.com/ipfs/go-ipfs/mount/providers/9P/filesystems/ipns"
	"github.com/ipfs/go-ipfs/mount/providers/9P/filesystems/mfs"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	coreoptions "github.com/ipfs/interface-go-ipfs-core/options"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

var _ p9.File = (*File)(nil)
var _ common.WalkRef = (*File)(nil)

var errKeyNotInStore = errors.New("requested key was not found in the key store")

// The KeyFS File exposes the IPFS API over a p9.File interface
// Walk does not expect a namespace, only path arguments
// e.g. `ipfs.Walk([]string("Qm...", "subdir")` not `ipfs.Walk([]string("ipfs", "Qm...", "subdir")`
type File struct {
	templatefs.NoopFile
	p9.DefaultWalkGetAttr

	common.CoreBase
	common.OverlayBase

	ents p9.Dirents

	// shared roots across all FS instances
	sharedLock    *sync.Mutex               // should be held when accessing the root map
	mroots        map[string]common.WalkRef // map["key"]*MFS{}
	parent, proxy common.WalkRef
}

func Attacher(ctx context.Context, core coreiface.CoreAPI, ops ...common.AttachOption) p9.Attacher {
	options := common.AttachOps(ops...)
	kd := &File{
		CoreBase:    common.NewCoreBase("/keyfs", core, ops...),
		OverlayBase: common.OverlayBase{ParentCtx: ctx},
		parent:      options.Parent,
		mroots:      make(map[string]common.WalkRef),
	}

	// non-keyed requests fall through to IPNS
	opts := []common.AttachOption{
		common.Parent(kd),
		common.Logger(logging.Logger("IPNS")),
	}

	subsystem, err := ipns.Attacher(ctx, core, opts...).Attach()
	if err != nil {
		panic(err)
	}

	kd.proxy = subsystem.(common.WalkRef)

	// detach from our proxied system when we fall out of memory
	runtime.SetFinalizer(kd, func(keyRoot *File) {
		keyRoot.proxy.Close()
	})

	return kd
}

func (kd *File) Attach() (p9.File, error) {
	kd.Logger.Debugf("Attach")

	newFid, err := kd.clone()
	if err != nil {
		return nil, err
	}

	newFid.FilesystemCtx, newFid.FilesystemCancel = context.WithCancel(newFid.ParentCtx)
	return newFid, nil
}

func (kd *File) Open(mode p9.OpenFlags) (p9.QID, uint32, error) {
	kd.Logger.Debugf("Open")

	qid, err := kd.QID()
	if err != nil {
		return p9.QID{}, 0, err
	}

	ctx, cancel := kd.CallCtx()
	defer cancel()

	ents, err := getKeys(ctx, kd.Core)
	if err != nil {
		kd.Logger.Errorf("Open hit: %s", err)
		return qid, 0, err
	}

	kd.ents = ents
	return qid, 0, nil
}

func (kd *File) Close() error {
	kd.Closed = true
	kd.ents = nil

	if kd.FilesystemCancel != nil {
		kd.FilesystemCancel()
	}

	if kd.OperationsCancel != nil {
		kd.OperationsCancel()
	}

	return nil
}

func (kd *File) Readdir(offset uint64, count uint32) (p9.Dirents, error) {
	kd.Logger.Debugf("Readdir")
	if kd.ents == nil {
		return nil, common.FileNotOpen
	}

	return common.FlatReaddir(kd.ents, offset, count)
}

/* WalkRef relevant */

func (kd *File) Fork() (common.WalkRef, error) {
	newFid, err := kd.clone()
	if err != nil {
		return nil, err
	}

	// make sure we were actually initalized
	if kd.FilesystemCtx == nil {
		return nil, common.FSCtxNotInitalized
	}

	// and also not canceled / still valid
	if err := kd.FilesystemCtx.Err(); err != nil {
		return nil, err
	}

	newFid.OperationsCtx, newFid.OperationsCancel = context.WithCancel(kd.FilesystemCtx)

	return newFid, nil

}

// KeyFS forks the IPFS root that was set during construction
// and calls step on it rather than itself
func (kd *File) Step(keyName string) (common.WalkRef, error) {
	callCtx, cancel := kd.CallCtx()
	defer cancel()

	key, err := getKey(callCtx, keyName, kd.Core)
	switch err {
	default:
		// unexpected failure
		return nil, err

	case errKeyNotInStore:
		// proxy non-keyed requests to an IPNS derivative
		proxyRef, err := kd.proxy.Fork()
		if err != nil {
			return nil, err
		}
		return proxyRef.Step(keyName)

	case nil:
		// appropriate roots that are names of keys we own
		mfsNode, ok := kd.mroots[keyName]
		if !ok {
			// init
			corePath, err := kd.Core.ResolvePath(callCtx, key.Path())
			if err != nil {
				return nil, err
			}
			//TODO: check key target's type; MFS for dirs, UnixIO for files

			mfsRootActual, err := common.CidToMFSRoot(kd.FilesystemCtx, corePath.Cid(), kd.Core,
				ipnsPublisher(key.Name(), offlineAPI(kd.Core).Name()))

			if err != nil {
				return nil, err
			}

			opts := []common.AttachOption{
				common.Parent(kd),
				common.MFSRoot(mfsRootActual),
				common.Logger(logging.Logger("IPNS-Key")),
			}

			mfsRootVirtual, err := mfs.Attacher(kd.FilesystemCtx, kd.Core, opts...).Attach()
			if err != nil {
				return nil, err
			}

			mfsNode = mfsRootVirtual.(common.WalkRef)
			kd.mroots[keyName] = mfsNode

			// TODO: validate this
			runtime.SetFinalizer(mfsNode, func(wr common.WalkRef) {
				delete(kd.mroots, keyName)
			})
		}

		return mfsNode, nil
	}
}

func (kd *File) CheckWalk() error {
	if kd.ents != nil {
		return common.FileOpen
	}
	return nil
}
func (kd *File) QID() (p9.QID, error) {
	return p9.QID{Type: p9.TypeDir,
		Path: common.CidToQIDPath(common.RootPath(kd.CoreNamespace).Cid()),
	}, nil
}
func (kd *File) Backtrack() (common.WalkRef, error) {
	if kd.parent != nil {
		return kd.parent, nil
	}
	return kd, nil
}

/* base class boilerplate */

func (kd *File) Walk(names []string) ([]p9.QID, p9.File, error) {
	kd.Logger.Debugf("Walk %q: %v", kd.String(), names)
	return common.Walker(kd, names)
}

func (kd *File) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	return p9.QID{
			Type: p9.TypeDir,
			Path: common.CidToQIDPath(common.RootPath(kd.CoreNamespace).Cid()),
		},
		p9.AttrMask{
			Mode: true,
		},
		p9.Attr{
			Mode: p9.ModeDirectory | common.IRXA | 0220,
		},
		nil
}

func getKeys(ctx context.Context, core coreiface.CoreAPI) (p9.Dirents, error) {
	keys, err := core.Key().List(ctx)
	if err != nil {
		return nil, err
	}

	ents := make(p9.Dirents, 0, len(keys))

	// temporary conversion storage
	attr := &p9.Attr{}
	requestType := p9.AttrMask{Mode: true}

	var offset uint64 = 1
	for _, key := range keys {
		//
		ipldNode, err := core.ResolveNode(ctx, key.Path())
		if err != nil {
			//FIXME: bug in either the CoreAPI, http client, or somewhere else
			//if err == coreiface.ErrResolveFailed {
			//HACK:
			if err.Error() == coreiface.ErrResolveFailed.Error() {
				continue // skip unresolvable keys (typical when a key exists but hasn't been published to
			}
			return nil, err
		}
		if _, err = common.IpldStat(ctx, attr, ipldNode, requestType); err != nil {
			return nil, err
		}

		ents = append(ents, p9.Dirent{
			//Name:   gopath.Base(key.Path().String()),
			Name:   gopath.Base(key.Name()),
			Offset: offset,
			QID: p9.QID{
				Type: attr.Mode.QIDType(),
				Path: common.CidToQIDPath(ipldNode.Cid()),
			},
		})
		offset++
	}
	return ents, nil
}

func ipnsPublisher(keyName string, nameAPI coreiface.NameAPI) func(context.Context, cid.Cid) error {
	return func(ctx context.Context, rootCid cid.Cid) error {
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		_, err := nameAPI.Publish(callCtx, corepath.IpfsPath(rootCid), coreoptions.Name.Key(keyName), coreoptions.Name.AllowOffline(true))
		return err
	}
}

func getKey(ctx context.Context, keyName string, core coreiface.CoreAPI) (coreiface.Key, error) {
	if keyName == "self" {
		return core.Key().Self(ctx)
	}

	keys, err := core.Key().List(ctx)
	if err != nil {
		return nil, err
	}

	var key coreiface.Key
	for _, curKey := range keys {
		if curKey.Name() == keyName {
			key = curKey
			break
		}
	}

	if key == nil {
		//return nil, fmt.Errorf(errFmtExternalWalk, keyName)
		return nil, errKeyNotInStore
	}

	return key, nil
}

func (kd *File) clone() (*File, error) {
	// make sure we were actually initalized
	if kd.ParentCtx == nil {
		return nil, common.FSCtxNotInitalized
	}

	// and also not canceled / still valid
	if err := kd.ParentCtx.Err(); err != nil {
		return nil, err
	}

	// all good; derive a new reference from this instance and return it
	return &File{
		CoreBase:    kd.CoreBase,
		OverlayBase: kd.OverlayBase.Clone(),
		parent:      kd.parent,
		proxy:       kd.proxy,
		mroots:      kd.mroots,
	}, nil
}

func offlineAPI(core coreiface.CoreAPI) coreiface.CoreAPI {
	oAPI, err := core.WithOptions(coreoptions.Api.Offline(true))
	if err != nil {
		panic(err)
	}
	return oAPI
}
