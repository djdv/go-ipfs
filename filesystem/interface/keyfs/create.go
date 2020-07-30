package keyfs

import (
	"context"
	"errors"

	"github.com/ipfs/go-ipfs/filesystem"
	fserrors "github.com/ipfs/go-ipfs/filesystem/errors"
	interfaceutils "github.com/ipfs/go-ipfs/filesystem/interface"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

func (ki *keyInterface) createSplit(path string) (self bool, remote filesystem.Interface, fsPath string, err error) {
	keyName, remainder := splitPath(path)
	if remainder == "" { // no subpath, request is for us
		self = true
		fsPath = keyName
		return
	}

	var coreKey coreiface.Key
	if coreKey, err = ki.checkKey(keyName); err != nil {
		err = &interfaceutils.Error{
			Cause: err,
			Type:  fserrors.NotExist,
		}
		return
	}

	if coreKey == nil { // the request was valid, but not for a key we own
		fsPath = path
		remote = ki.ipns // let the host fs handle the requested operation
		return
	}

	remote, err = ki.getRoot(coreKey)
	if err != nil {
		return
	}

	fsPath = remainder
	return
}

func (ki *keyInterface) Make(path string) error {
	self, remote, fsPath, err := ki.createSplit(path)
	if err != nil {
		return err
	}

	if self {
		return ki.makeEmptyKey(coreiface.TFile, fsPath)
	}

	return remote.Make(fsPath)
}

func (ki *keyInterface) MakeDirectory(path string) error {
	self, remote, fsPath, err := ki.createSplit(path)
	if err != nil {
		return err
	}

	if self {
		return ki.makeEmptyKey(coreiface.TDirectory, fsPath)
	}

	return remote.MakeDirectory(fsPath)
}

func (ki *keyInterface) MakeLink(path, linkTarget string) error {
	self, remote, fsPath, err := ki.createSplit(path)
	if err != nil {
		return err
	}

	if self {
		callCtx, cancel := interfaceutils.CallContext(ki.ctx)
		defer cancel()
		linkNode, err := makeLinkNode(callCtx, ki.core.Dag(), linkTarget)
		if err != nil {
			return err
		}

		if err := makeKeyWithNode(callCtx, ki.core, fsPath, linkNode); err != nil {
			return err
		}

		return localPublish(callCtx, ki.core, fsPath, corepath.IpfsPath(linkNode.Cid()))
	}

	return remote.MakeLink(fsPath, linkTarget)
}

func (ki *keyInterface) makeEmptyKey(nodeType coreiface.FileType, keyName string) error {
	callCtx, cancel := interfaceutils.CallContext(ki.ctx)
	defer cancel()

	nodeFoundation, err := makeEmptyNode(callCtx, ki.core.Dag(), nodeType)
	if err != nil {
		return err
	}

	if err := makeKeyWithNode(callCtx, ki.core, keyName, nodeFoundation); err != nil {
		return err
	}

	return localPublish(callCtx, ki.core, keyName, corepath.IpfsPath(nodeFoundation.Cid()))
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
		return nil, &interfaceutils.Error{
			Cause: errors.New("unexpected node type"),
			Type:  fserrors.Other,
		}
	}

	// push it to the datastore
	if err := dagAPI.Add(ctx, node); err != nil {
		return nil, &interfaceutils.Error{
			Cause: err,
			Type:  fserrors.IO,
		}
	}

	return node, nil
}

func makeKeyWithNode(ctx context.Context, core coreiface.CoreAPI, keyName string, node ipld.Node) error {
	if _, err := core.Key().Generate(ctx, keyName); err != nil {
		return &interfaceutils.Error{
			Cause: err,
			Type:  fserrors.IO,
		}
	}

	if err := core.Dag().Add(ctx, node); err != nil {
		return &interfaceutils.Error{
			Cause: err,
			Type:  fserrors.IO,
		}
	}

	return nil
}

func makeLinkNode(ctx context.Context, dagAPI coreiface.APIDagService, linkTarget string) (ipld.Node, error) {
	dagData, err := unixfs.SymlinkData(linkTarget)
	if err != nil {
		return nil, &interfaceutils.Error{
			Cause: err,
			Type:  fserrors.Other,
		}
	}

	dagNode := dag.NodeWithData(dagData)
	// TODO: use raw node with raw codec and tiny blake hash (after testing the standard)
	// symlinks shouldn't be big enough to warrant anything else
	//dagNode := dag.NewRawNodeWPrefix(dagData, cid.V1Builder{Codec: cid.Raw, MhType: mh.BLAKE2S_MIN})
	//dagNode.SetCidBuilder(cid.V1Builder{Codec: cid.DagCBOR, MhType: mh.SHA2_256})

	// push it to the datastore
	if err := dagAPI.Add(ctx, dagNode); err != nil {
		return nil, &interfaceutils.Error{
			Cause: err,
			Type:  fserrors.IO,
		}
	}
	return dagNode, nil
}
