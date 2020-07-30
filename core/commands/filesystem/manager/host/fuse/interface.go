//+build !nofuse

package fuse

import (
	"context"
	gopath "path"
	"strings"
	"sync"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/options"
	"github.com/ipfs/go-ipfs/filesystem"
	logging "github.com/ipfs/go-log"
)

// TODO: how do we do this; without a dependency loop? [6c751cf6-1fb1-4893-8a31-8f9d20b4c38c]
// (regardless of type `manager.const string` or `manager.Stringer`)
//const logGroup = manager.Fuse
const logGroup = "FUSE"

// fuseMounter mounts requests in the host FS via the FUSE API
type fuseMounter struct {
	ctx context.Context // TODO: needs to trigger close on cancel
	sync.Mutex
	log logging.EventLogger

	// FS provider
	fuseInterface fuselib.FileSystemInterface // the actual interface with the host
}

func NewMounter(ctx context.Context, fs filesystem.Interface, opts ...options.Option) (Mounter, error) {
	settings := options.Parse(opts...)

	fsi, err := NewFuseInterface(fs, opts...)
	if err != nil {
		return nil, err
	}

	return &fuseMounter{
		ctx: ctx,
		log: logging.Logger(gopath.Join(
			strings.ToLower(settings.LogPrefix), // (opt)fmt: `filesystem`
			strings.ToLower(logGroup),           // fmt: `fuse`
		)),
		fuseInterface: fsi,
	}, nil
}

func NewFuseInterface(fs filesystem.Interface, opts ...options.Option) (fuselib.FileSystemInterface, error) {
	settings := options.Parse(opts...)

	fuseInterface := &hostBinding{
		nodeInterface: fs,
		log: logging.Logger(gopath.Join(
			settings.LogPrefix,                 // (opt)fmt: `filesystem`
			strings.ToLower(logGroup),          // fmt: `fuse`
			strings.ToLower(fs.ID().String())), // fmt: `ipfs`
		),
	}

	// TODO: should we switch on the ID per API or have a node.Option for
	// swapping out methods like Stat(name), Permission(name), etc.
	// so that plexed attachers are possible
	// e.g. `permission("/x")` => not writable;
	// `permission("/x/y")` => writable
	// or, always using cached metadata for read only systems
	// etc.
	switch fs.ID() {
	case filesystem.PinFS, filesystem.IPFS:
		fuseInterface.readdirplusGen = staticStat
	case filesystem.KeyFS, filesystem.Files:
		fuseInterface.filesWritable = true
		fallthrough
	default:
		fuseInterface.readdirplusGen = dynamicStat
	}

	return fuseInterface, nil
}
