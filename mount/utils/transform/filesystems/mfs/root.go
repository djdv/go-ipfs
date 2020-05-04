package mfs

import (
	"context"
	"fmt"
	"time"

	"github.com/ipfs/go-cid"
	dag "github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	coreoptions "github.com/ipfs/interface-go-ipfs-core/options"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

// TODO: lint in 9P [5c18e17f-3e9d-490a-b8c4-5a78ab067678]

func PathToMFSRoot(ctx context.Context, path corepath.Path, core coreiface.CoreAPI, publish mfs.PubFunc) (*mfs.Root, error) {
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ipldNode, err := core.ResolveNode(callCtx, path)
	if err != nil {
		return nil, err
	}

	pbNode, ok := ipldNode.(*dag.ProtoNode)
	if !ok {
		return nil, fmt.Errorf("%q has incompatible type %T", path.String(), ipldNode)
	}

	return mfs.NewRoot(ctx, core.Dag(), pbNode, publish)
}

func IPNSPublisher(keyName string, nameAPI coreiface.NameAPI) func(context.Context, cid.Cid) error {
	return func(ctx context.Context, rootCid cid.Cid) error {
		callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		_, err := nameAPI.Publish(callCtx, corepath.IpfsPath(rootCid), coreoptions.Name.Key(keyName), coreoptions.Name.AllowOffline(true))
		return err
	}
}
