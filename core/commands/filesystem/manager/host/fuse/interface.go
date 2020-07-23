//+build !nofuse

package fuse

import (
	"context"
	gopath "path"
	"sync"

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/options"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
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

func NewHostInterface(fs filesystem.Interface, opts ...options.Option) fuselib.FileSystemInterface {
	settings := options.Parse(opts...)

	fuseInterface := &nodeBinding{
		nodeInterface: fs,
		log: logging.Logger(gopath.Join(
			settings.LogPrefix, // fmt: `filesystem`
			logGroup,           // fmt: `FUSE`
			fs.ID().String()),  // fmt: `IPFS`
		),
		//initSignal: settings.InitSignal,
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

	return fuseInterface
}

func HostMounter(ctx context.Context, fs filesystem.Interface, opts ...options.Option) (Mounter, error) {
	settings := options.Parse(opts...)

	return &fuseMounter{
		ctx: ctx,
		log: logging.Logger(gopath.Join(
			settings.LogPrefix,
			fs.ID().String(), // fmt: `IPFS`|`IPNS`|...
		)),
		fuseInterface: NewHostInterface(fs, opts...),
	}, nil
}
