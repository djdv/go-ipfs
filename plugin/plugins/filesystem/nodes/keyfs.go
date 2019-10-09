package fsnodes

import (
	"context"

	"github.com/djdv/p9/p9"
	nodeopts "github.com/ipfs/go-ipfs/plugin/plugins/filesystem/nodes/options"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var _ p9.File = (*KeyFS)(nil)
var _ walkRef = (*KeyFS)(nil)

type KeyFS struct {
	IPFSBase
}

func KeyFSAttacher(ctx context.Context, core coreiface.CoreAPI, ops ...nodeopts.AttachOption) p9.Attacher {
	kd := &KeyFS{IPFSBase: newIPFSBase(ctx, "/keyfs", core, ops...)}
	kd.Qid.Type = p9.TypeDir
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

	kd.proxy = subsystem.(walkRef)

	return kd
}

func (kd *KeyFS) Derive() walkRef {
	newFid := &KeyFS{
		IPFSBase: kd.IPFSBase.Derive(),
	}
	return newFid
}

func (kd *KeyFS) Attach() (p9.File, error) {
	kd.Logger.Debugf("Attach")
	return kd, nil
}

func (kd *KeyFS) Step(keyName string) (walkRef, error) {
	// proxy the request for "keyName" to IPFS root (set on us during construction)
	return kd.proxy.Step(keyName)
}

func (kd *KeyFS) Walk(names []string) ([]p9.QID, p9.File, error) {
	kd.Logger.Debugf("Walk names %v", names)
	kd.Logger.Debugf("Walk myself: %v", kd.Qid)

	return walker(kd, names)
}

func (kd *KeyFS) Backtrack() (walkRef, error) {
	// return our parent, or ourselves if we don't have one
	if kd.parent != nil {
		return kd.parent, nil
	}
	return kd, nil
}

// temporary stub to allow forwarding requests on empty directory
func (kd *KeyFS) Readdir(offset uint64, count uint32) ([]p9.Dirent, error) {
	return nil, nil
}
