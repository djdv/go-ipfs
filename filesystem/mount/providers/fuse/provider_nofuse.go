//+build nofuse

package fuse

import (
	"context"
	"errors"

	mountinter "github.com/ipfs/go-ipfs/filesystem/mount"
	provcom "github.com/ipfs/go-ipfs/filesystem/mount/providers"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

// NOTE: don't export this for `!nofuse` build constraints, things depending on this should fail at compile time
// (specifically our tests)
var ErrNoFuse = errors.New(`binary was built without fuse support ("nofuse" tag provided during build)`)

func NewProvider(context.Context, mountinter.Namespace, string, coreiface.CoreAPI, ...provcom.Option) (mountinter.Provider, error) {
	return nil, ErrNoFuse
}
