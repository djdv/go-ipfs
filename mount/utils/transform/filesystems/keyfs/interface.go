package keyfs

import (
	"context"

	"github.com/ipfs/go-cid"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	transcom "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/ipfscore"
	"github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/ufs"
	ipld "github.com/ipfs/go-ipld-format"
	gomfs "github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

// TODO: docs
type keyInterface struct {
	ctx  context.Context
	core transcom.CoreExtender

	ufs ufs.UFS // `File` constructor

	// keys as files
	/*
		fileTable   fileTable  // the table which holds (shared) `File` references
		fileTableMu sync.Mutex // guards table access

		// keys as file systems
		fileSystemTable   fileSystemTable // the table which holds (shared) `Interface` references
		fileSystemTableMu sync.Mutex      // guards table access
	*/

	references referenceTable

	ipns transform.Interface // any requests to keys we don't own get proxied to ipns
}

// TODO: docs
func NewInterface(ctx context.Context, core coreiface.CoreAPI) transform.Interface {
	return &keyInterface{
		ctx:        ctx,
		core:       &transcom.CoreExtended{core},
		ufs:        ufs.NewInterface(ctx, core),
		ipns:       ipfscore.NewInterface(ctx, core, mountinter.NamespaceIPNS),
		references: newReferenceTable(),
	}
}

func (ki *keyInterface) Close() error {
	// TODO: cascade close for all open references
	return nil
}

// TODO: having both of these is dumb; do something about it
func (ki *keyInterface) publisherGenUFS(keyName string) ufs.ModifiedFunc {
	return func(nd ipld.Node) error {
		return localPublish(ki.ctx, ki.core, keyName, corepath.IpfsPath(nd.Cid()))
	}
}

// TODO: having both of these is dumb; do something about it
func (ki *keyInterface) publisherGenMFS(keyName string) gomfs.PubFunc {
	return func(ctx context.Context, cid cid.Cid) error {
		return localPublish(ctx, ki.core, keyName, corepath.IpfsPath(cid))
	}
}

// TODO: test this; quick port for now
// cross FS requests are going to be busted if we get them (/key -> /other-key/newhome)
func (ki *keyInterface) Rename(oldName, newName string) error {
	keyName, remainder := splitPath(oldName)
	if remainder == "" { // rename on key itself
		callCtx, cancel := transcom.CallContext(ki.ctx)
		defer cancel()
		_, _, err := ki.core.Key().Rename(callCtx, keyName, newName[1:])
		if err != nil {
			return &transcom.Error{
				Cause: err,
				Type:  transform.ErrorIO,
			}
		}
		return nil
	}

	// subrequest
	fs, _, _, deferFunc, err := ki.selectFS(oldName)
	if err != nil {
		return err
	}
	defer deferFunc()

	_, subNewName := splitPath(newName)

	return fs.Rename(remainder, subNewName)
}
