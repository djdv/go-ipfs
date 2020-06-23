package ufs

import (
	"context"
	"io"

	chunk "github.com/ipfs/go-ipfs-chunker"
	"github.com/ipfs/go-ipfs/filesystem"
	interfaceutils "github.com/ipfs/go-ipfs/filesystem/interfaces"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-unixfs/mod"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

type ufsInterface struct {
	ctx              context.Context
	core             interfaceutils.CoreExtender
	modifiedCallback ModifiedFunc
}

type (
	// UFS extends the file system `Interface`
	// allowing the caller to set a callback used by the system
	// This callback is attached to a `File` before being returned in `Open`
	// and called during operations that modify said `File`
	// it is valid to reset this value to `nil`
	UFS interface {
		filesystem.Interface
		SetModifier(ModifiedFunc)
	}

	ModifiedFunc func(ipld.Node) error
)

func NewInterface(ctx context.Context, core coreiface.CoreAPI) UFS {
	return &ufsInterface{
		ctx:  ctx,
		core: &interfaceutils.CoreExtended{core},
	}
}

func (ui *ufsInterface) SetModifier(callback ModifiedFunc) { ui.modifiedCallback = callback }

// TODO: stale docs
// Open will either grab an existing dag modifier and wrap it as a keyFile
// or construct a dag modifier and do the same
// handling reference count internally/automatically via keyFile's `Close` method
// TODO: parse flags and limit functionality contextually (RO, WO, etc.)
// for now we always give full access
func (ui *ufsInterface) Open(path string, _ filesystem.IOFlags) (filesystem.File, error) {
	callCtx, cancel := interfaceutils.CallContext(ui.ctx)
	defer cancel()
	ipldNode, err := ui.core.ResolveNode(callCtx, corepath.New(path))
	if err != nil {
		return nil, err
	}

	dmod, err := mod.NewDagModifier(ui.ctx, ipldNode, ui.core.Dag(), func(r io.Reader) chunk.Splitter {
		return chunk.NewBuzhash(r) // TODO: maybe switch this back to the default later; buzhash should be faster so we're keeping it temporarily while testing
	})
	if err != nil {
		return nil, &interfaceutils.Error{Cause: err, Type: filesystem.ErrorOther}
	}

	return &dagRef{DagModifier: dmod, modifiedCallback: ui.modifiedCallback}, nil
}

func (*ufsInterface) Close() error { return nil }