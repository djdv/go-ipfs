//+build nofuse

package fuse

import (
	"context"
	"errors"

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"
	"github.com/ipfs/go-ipfs/filesystem"
)

// TODO: interface changed
// we need to add errors in for these

// NOTE: don't export this for `!nofuse` build constraints, things depending on this should fail at compile time
// (specifically our tests)
var ErrNoFuse = errors.New(`binary was built without fuse support ("nofuse" tag provided during build)`)

func HostAttacher(context.Context, filesystem.Interface) host.Attacher {
	panic(ErrNoFuse)
}

func ParseRequest(host.Request) (string, string) {
	panic(ErrNoFuse)
}
