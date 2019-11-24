package fsnodes

import (
	"context"

	"github.com/hugelgupf/p9/p9"
	"github.com/hugelgupf/p9/unimplfs"
	nodeopts "github.com/ipfs/go-ipfs/plugin/plugins/filesystem/nodes/options"
	fsutils "github.com/ipfs/go-ipfs/plugin/plugins/filesystem/utils"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var _ p9.File = (*KeyFS)(nil)
var _ fsutils.WalkRef = (*KeyFS)(nil)

type KeyFS struct {
	unimplfs.NoopFile
	p9.DefaultWalkGetAttr

	IPFSBase
}

func KeyFSAttacher(ctx context.Context, core coreiface.CoreAPI, ops ...nodeopts.AttachOption) p9.Attacher {
	kd := &KeyFS{IPFSBase: newIPFSBase(ctx, "/keyfs", core, ops...)}
	kd.qid.Type = p9.TypeDir
	kd.meta.Mode, kd.metaMask.Mode = p9.ModeDirectory|IRXA|0220, true

	// non-keyed requests fall through to IPNS
	opts := []nodeopts.AttachOption{
		nodeopts.Parent(kd),
		nodeopts.Logger(logging.Logger("IPNS")),
	}

	subsystem, err := IPNSAttacher(ctx, core, opts...).Attach()
	if err != nil {
		panic(err)
	}

	kd.proxy = subsystem.(fsutils.WalkRef)

	return kd
}

// this root has no paths to walk; forking anythign besides the fs doesn't make sense for us
func (kd *KeyFS) clone() (fsutils.WalkRef, error) {
	newFid := &KeyFS{IPFSBase: kd.IPFSBase.clone()}
	// set new fs context
	if err := newFid.forkFilesystem(); err != nil {
		return nil, err
	}
	return newFid, nil
}

func (kd *KeyFS) Attach() (p9.File, error) {
	kd.Logger.Debugf("Attach")
	return kd.clone()
}

func (kd *KeyFS) Open(mode p9.OpenFlags) (p9.QID, uint32, error) { return *kd.qid, 0, nil }
func (kd *KeyFS) Close() error                                   { return kd.IPFSBase.close() }

// temporary stub to allow forwarding requests on empty directory
// will contain keys later
func (kd *KeyFS) Readdir(offset uint64, count uint32) ([]p9.Dirent, error) {
	return nil, nil
}

/* WalkRef relevant */

func (kd *KeyFS) Fork() (fsutils.WalkRef, error) {
	return kd.clone()
}

// KeyFS forks the IPFS root that was set during construction
// and calls step on it rather than itself
func (kd *KeyFS) Step(name string) (fsutils.WalkRef, error) {
	newFid, err := kd.proxy.Fork()
	if err != nil {
		return nil, err
	}
	return newFid.Step(name)
}

func (kd *KeyFS) CheckWalk() error                    { return kd.Base.checkWalk() }
func (kd *KeyFS) QID() (p9.QID, error)                { return kd.Base.qID() }
func (kd *KeyFS) Backtrack() (fsutils.WalkRef, error) { return kd.IPFSBase.backtrack(kd) }

/* base class boilerplate */

func (kd *KeyFS) Walk(names []string) ([]p9.QID, p9.File, error) {
	kd.Logger.Debugf("Walk names %v", names)
	kd.Logger.Debugf("Walk myself: %v", kd.qid)

	return fsutils.Walker(kd, names)
}

func (kd *KeyFS) GetAttr(req p9.AttrMask) (p9.QID, p9.AttrMask, p9.Attr, error) {
	return kd.Base.getAttr(req)
}
