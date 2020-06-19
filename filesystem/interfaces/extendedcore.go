package transformcommon

import (
	"context"
	"fmt"
	"time"

	files "github.com/ipfs/go-ipfs-files"
	transform "github.com/ipfs/go-ipfs/filesystem"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-unixfs"
	unixpb "github.com/ipfs/go-unixfs/pb"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

const callTimeout = 20 * time.Second

func CallContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, callTimeout)
}

type CoreExtender interface {
	coreiface.CoreAPI
	// Stat takes in a path and a list of desired attributes for the object residing at that path
	// Along with the container of values,
	// it returns a list of attributes which were populated
	// Stat is not gauranteed to return the request exactly
	// it may contain more or less information than was requested
	// Thus, it is the callers responsability to inspect the returned list
	// to see if values they require were in fact populated
	// (this is due to the fact that the referenced objects
	// may not implement the constructs requested)
	Stat(context.Context, corepath.Path, transform.IPFSStatRequest) (*transform.IPFSStat, transform.IPFSStatRequest, error)
	// ExtractLink takes in a path to a link and returns the string it contains
	ExtractLink(corepath.Path) (string, error)
}

type CoreExtended struct{ coreiface.CoreAPI }

func (core *CoreExtended) Stat(ctx context.Context, path corepath.Path, req transform.IPFSStatRequest) (*transform.IPFSStat, transform.IPFSStatRequest, error) {
	ipldNode, err := core.ResolveNode(ctx, path)
	if err != nil {
		return nil, transform.IPFSStatRequest{}, err
	}

	switch typedNode := ipldNode.(type) {
	case *dag.ProtoNode:
		ufsNode, err := unixfs.ExtractFSNode(typedNode)
		if err != nil {
			return nil, transform.IPFSStatRequest{}, &Error{
				Cause: err,
				Type:  transform.ErrorOther,
			}
		}
		return unixFSAttr(ufsNode, req)

	// pretend Go allows this:
	// case *dag.RawNode, *cbor.Node:
	// fallthrough
	default:
		return genericAttr(typedNode, req)
	}
}
func (core *CoreExtended) ExtractLink(path corepath.Path) (string, error) {
	// make sure the path is actually a link
	callCtx, cancel := CallContext(context.Background())
	defer cancel()
	iStat, _, err := core.Stat(callCtx, path, transform.IPFSStatRequest{Type: true})
	if err != nil {
		return "", err
	}

	if iStat.FileType != coreiface.TSymlink {
		return "", &Error{
			Cause: fmt.Errorf("%q is not a symlink", path.String()),
			Type:  transform.ErrorInvalidItem,
		}
	}

	// if it is, read it
	linkNode, err := core.Unixfs().Get(callCtx, path)
	if err != nil {
		return "", &Error{
			Cause: err,
			Type:  transform.ErrorIO,
		}
	}

	// NOTE: the implementation of this does no type checks [2020.06.04]
	// which is why we check the node's type above
	return files.ToSymlink(linkNode).Target, nil
}

// implemented just for the error wrapping
func (core *CoreExtended) ResolveNode(ctx context.Context, path corepath.Path) (ipld.Node, error) {
	n, err := core.CoreAPI.ResolveNode(ctx, path)
	if err != nil {
		// TODO: inspect error to disambiguate type
		return nil, &Error{
			Cause: err,
			Type:  transform.ErrorNotExist,
		}
	}
	return n, nil
}

// implemented just for the error wrapping
func (core *CoreExtended) ResolvePath(ctx context.Context, path corepath.Path) (corepath.Resolved, error) {
	p, err := core.CoreAPI.ResolvePath(ctx, path)
	if err != nil {
		// TODO: inspect error to disambiguate type
		return nil, &Error{
			Cause: err,
			Type:  transform.ErrorNotExist,
		}
	}
	return p, nil
}

func genericAttr(genericNode ipld.Node, req transform.IPFSStatRequest) (*transform.IPFSStat, transform.IPFSStatRequest, error) {
	var (
		attr        = new(transform.IPFSStat)
		filledAttrs transform.IPFSStatRequest
	)

	if req.Type {
		// raw nodes only contain data so we'll treat them as a flat file
		// cbor nodes are not currently supported via UnixFS so we assume them to contain only data
		// TODO: review ^ is there some way we can implement this that won't blow up in the future?
		// (if unixfs supports cbor and directories are implemented to use them )
		attr.FileType, filledAttrs.Type = coreiface.TFile, true
	}

	if req.Size || req.Blocks {
		nodeStat, err := genericNode.Stat()
		if err != nil {
			return attr, filledAttrs, &Error{
				Cause: err,
				Type:  transform.ErrorIO,
			}
		}

		if req.Size {
			attr.Size, filledAttrs.Size = uint64(nodeStat.CumulativeSize), true
		}

		if req.Blocks {
			attr.BlockSize, filledAttrs.Blocks = uint64(nodeStat.BlockSize), true
		}
	}

	return attr, filledAttrs, nil
}

// returns attr, filled members, error
func unixFSAttr(ufsNode *unixfs.FSNode, req transform.IPFSStatRequest) (*transform.IPFSStat, transform.IPFSStatRequest, error) {
	var (
		attr        transform.IPFSStat
		filledAttrs transform.IPFSStatRequest
	)

	if req.Type {
		attr.FileType, filledAttrs.Type = unixfsTypeToCoreType(ufsNode.Type()), true
	}

	if req.Blocks {
		// NOTE: we can't account for variable block size so we use the size of the first block only (if any)
		blocks := len(ufsNode.BlockSizes())
		if blocks > 0 {
			attr.BlockSize = ufsNode.BlockSize(0)
			attr.Blocks = uint64(blocks)
		}

		// 0 is a valid value for these fields, especially for non-regular files
		// so set this to true regardless of if one was provided or not
		filledAttrs.Blocks = true
	}

	if req.Size {
		attr.Size, filledAttrs.Size = ufsNode.FileSize(), true
	}

	// TODO [eventually]: handle time metadata in new UFS format standard

	return &attr, filledAttrs, nil
}

func unixfsTypeToCoreType(ut unixpb.Data_DataType) coreiface.FileType {
	switch ut {
	case unixpb.Data_Directory, unixpb.Data_HAMTShard:
		return coreiface.TDirectory
	case unixpb.Data_Symlink:
		return coreiface.TSymlink
	case unixpb.Data_File, unixpb.Data_Raw:
		return coreiface.TFile
	default:
		return coreiface.TUnknown
	}
}
