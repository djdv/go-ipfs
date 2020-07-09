//+build !nofuse

package fuse

import (
	"context"
	"runtime"
	"strings"

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"
	"github.com/ipfs/go-ipfs/filesystem"
)

// NOTE: [b7952c54-1614-45ea-a042-7cfae90c5361] cgofuse only supports ReaddirPlus on Windows
// if this ever changes (bumps libfuse from 2.8 -> 3.X+), add platform support here (and to any other tags with this UUID)
// TODO: this would be best in the fuselib itself; make a patch upstream
const canReaddirPlus bool = runtime.GOOS == "windows"

func newFuseHost(fs filesystem.Interface, opts ...Option) *fuseInterface {
	logName := strings.ToLower(fs.ID().String())
	settings := parseOptions(maybeAppendLog(opts, logName)...)

	// TODO: read-only option
	// always use cached items if available - otherwise assume data may change between calls

	fuseInterface := &fuseInterface{
		nodeInterface: fs,
		log:           settings.log,
		//initSignal:    make(InitSignal),
	}

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

// TODO: options
func HostAttacher(ctx context.Context, fs filesystem.Interface) host.Attacher {
	// TODO opts: settings := parseOptions(maybeAppendLog(nil, LogGroup+strings.ToLower(fuseInterface.ID().String())...)
	settings := parseOptions(maybeAppendLog(nil, LogGroup)...)

	return &fuseAttacher{
		log:           settings.log,
		ctx:           ctx,
		fuseInterface: newFuseHost(fs),
		//initSignal:    initSignal,
	}

	// TODO: InitSignal needs to become a pair and names need to be changed
	// the option should provide 2 channels, 1 for init/open, and 1 for destroy/close
	// so that the FS can signal when it's done starting and stopping, as well as provide the context of those ops
	// line semantics:
	// init line should be restricted to (m)exclusive access, half-duplex
	// e.g. 1 instantiation means there will be 1 expected reader/writer of a single message at a time
	// no possibility of cross talk should be allowed
	//initSignal := make(InitSignal)
	//	systemOpts := []Option{WithInitSignal(initSignal)}
	// TODO: WithResourceLock(options.resourceLock) when that's implemented

	//fsCtx, cancel := context.WithCancel(ctx)
}
