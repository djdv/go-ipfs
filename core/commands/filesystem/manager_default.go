//go:build !nofuse
// +build !nofuse

package fscmds

// TODO: dedupe bazil build conditions
// we need to handle `nofuse` and others like maybe just `bazilfuse` which adds support
// or have `cgofuse` + `fusecombined` for both
// in any case, cascade the generation
// ... => maybeAddFuse(dispath) =>
/*
func NewNodeInterface(ctx context.Context, node *core.IpfsNode) (manager.Interface, error) {
	cd := &commandDispatcher{
		index:    newIndex(),
		dispatch: make(dispatchMap),
		IpfsNode: node,
	}

	for _, api := range []filesystem.ID{
		filesystem.IPFS,
		filesystem.IPNS,
	} {
		fsb, err := bazil.NewBinder(ctx, api, node, false) // TODO: pull option from config
		if err != nil {
			return nil, err
		}
		cd.dispatch[requestHeader{API: filesystem.Fuse, ID: api}] = fsb
	}

	return cd, nil
}
*/
