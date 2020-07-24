//+build nofuse

package fuse

import (
	"context"
	"errors"

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/options"
	"github.com/ipfs/go-ipfs/filesystem"
)

// NOTE: don't export this for `!nofuse` build constraints, things depending on this should fail at compile time
// (specifically our tests)
var ErrNoFuse = errors.New(`binary was built without fuse support ("nofuse" tag provided during build)`)

func HostMounter(context.Context, filesystem.Interface, ...options.Option) (Mounter, error) {
	return nil, ErrNoFuse
}
