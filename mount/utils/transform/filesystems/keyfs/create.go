package keyfs

import (
	"context"
	"errors"

	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

func Mknod(ctx context.Context, core coreiface.CoreAPI, keyName string) error {
	return makeEmptyKey(ctx, coreiface.TFile, core, keyName)
}

func Mkdir(ctx context.Context, core coreiface.CoreAPI, keyName string) error {
	return makeEmptyKey(ctx, coreiface.TDirectory, core, keyName)
}

func Symlink(ctx context.Context, core coreiface.CoreAPI, keyName string, linkTarget string) error {
	linkNode, err := makeLinkNode(ctx, core.Dag(), linkTarget)
	if err != nil {
		return err
	}

	if err := makeKeyWithNode(ctx, core, keyName, linkNode); err != nil {
		return err
	}

	return localPublish(ctx, core, keyName, corepath.IpldPath(linkNode.Cid()))
}

func makeEmptyKey(ctx context.Context, nodeType coreiface.FileType, core coreiface.CoreAPI, keyName string) error {
	nodeFoundation, err := makeEmptyNode(ctx, core.Dag(), nodeType)
	if err != nil {
		return err
	}

	if err := makeKeyWithNode(ctx, core, keyName, nodeFoundation); err != nil {
		return err
	}

	return localPublish(ctx, core, keyName, corepath.IpldPath(nodeFoundation.Cid()))
}

func makeEmptyNode(ctx context.Context, dagAPI coreiface.APIDagService, nodeType coreiface.FileType) (ipld.Node, error) {
	var node ipld.Node

	// make the node in memory
	switch nodeType {
	case coreiface.TFile:
		node = dag.NodeWithData(unixfs.FilePBData(nil, 0))

	case coreiface.TDirectory:
		node = unixfs.EmptyDirNode()
	default:
		return nil, errors.New("unexpected node type")
	}

	// push it to the datastore
	if err := dagAPI.Add(ctx, node); err != nil {
		return nil, err
	}

	return node, nil
}

func makeKeyWithNode(ctx context.Context, core coreiface.CoreAPI, keyName string, node ipld.Node) error {
	if _, err := core.Key().Generate(ctx, keyName); err != nil {
		return err
	}

	if err := core.Dag().Add(ctx, node); err != nil {
		return err
	}

	return nil
}

func makeLinkNode(ctx context.Context, dagAPI coreiface.APIDagService, linkTarget string) (ipld.Node, error) {
	dagData, err := unixfs.SymlinkData(linkTarget)
	if err != nil {
		return nil, err
	}

	// TODO: use raw node with raw codec and tiny blake hash (after testing the standard)
	// symlinks shouldn't be big enough to warrant anything else
	// dagNode := dag.NewRawNodeWPrefix(dagData, cid.V1Builder{Codec: cid.Raw, MhType: mh.BLAKE2S_MIN})
	dagNode := dag.NodeWithData(dagData)
	//dagNode.SetCidBuilder(cid.V1Builder{Codec: cid.DagCBOR, MhType: mh.SHA2_256})

	// push it to the datastore
	if err := dagAPI.Add(ctx, dagNode); err != nil {
		return nil, err
	}
	return dagNode, nil
}
