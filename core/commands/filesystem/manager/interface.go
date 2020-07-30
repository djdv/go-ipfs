package manager

import (
	"context"
	"fmt"
	"sync"

	p9fsp "github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/9p"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/fuse"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/options"
	"github.com/ipfs/go-ipfs/filesystem"
	"github.com/ipfs/go-ipfs/filesystem/interface/ipfscore"
	"github.com/ipfs/go-ipfs/filesystem/interface/keyfs"
	"github.com/ipfs/go-ipfs/filesystem/interface/mfs"
	"github.com/ipfs/go-ipfs/filesystem/interface/pinfs"
	logging "github.com/ipfs/go-log"
	gomfs "github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

func NewDispatcher(ctx context.Context, core coreiface.CoreAPI, filesAPIRoot *gomfs.Root) (Dispatcher, error) {
	// TODO: reconsider defaults; we probably want to hostAttach in the foreground by default, not in the background
	//systemSettings := parseOptions(maybeAppendLog(opts, filesystem.LogGroup)...)

	// TODO: break this up
	systems := make(map[filesystem.ID]filesystem.Interface)
	var systemsMu sync.Mutex

	getFS := func(id filesystem.ID) (system filesystem.Interface, err error) {
		systemsMu.Lock()
		defer systemsMu.Unlock()
		var ok bool
		system, ok = systems[id]
		if ok {
			return
		}

		system, err = NewFileSystem(ctx, id, core, filesAPIRoot)
		if err != nil {
			systems[id] = system
		}

		return
	}

	hosts := make(map[API]interface{})
	var hostsMu sync.Mutex
	getHostAttacher := func(api API, fs filesystem.Interface) (hostMethod interface{}, err error) {
		hostsMu.Lock()
		defer hostsMu.Unlock()
		var ok bool
		hostMethod, ok = hosts[api]
		if ok {
			return
		}

		hostMethod, err = newHostAttacher(ctx, api, fs)
		if err != nil {
			hosts[api] = hostMethod
		}

		return
	}

	return &apiMux{
		ctx:             ctx,
		log:             logging.Logger(filesystem.LogGroup),
		NameIndex:       NewNameIndex(),
		getFS:           getFS,
		getHostAttacher: getHostAttacher,
	}, nil
}

func NewFileSystem(ctx context.Context, sysID filesystem.ID, core coreiface.CoreAPI, filesAPIRoot *gomfs.Root) (fs filesystem.Interface, err error) {
	switch sysID {
	case filesystem.PinFS:
		fs = pinfs.NewInterface(ctx, core)
	case filesystem.KeyFS:
		fs = keyfs.NewInterface(ctx, core)
	case filesystem.IPFS, filesystem.IPNS:
		fs = ipfscore.NewInterface(ctx, core, sysID)
	case filesystem.Files:
		fs, err = mfs.NewInterface(ctx, filesAPIRoot)
	default:
		err = fmt.Errorf("unknown nineAttacher requested: %v", sysID)
	}

	return
}

func newHostAttacher(ctx context.Context, api API, fs filesystem.Interface) (attacher interface{}, err error) {
	hostOpts := []options.Option{
		options.WithLogPrefix(filesystem.LogGroup), // fmt: `filesystem`
	}

	switch api {
	case Fuse:
		attacher, err = fuse.NewMounter(ctx, fs, hostOpts...)
	case Plan9Protocol:
		attacher, err = p9fsp.NewAttacher(ctx, fs, hostOpts...)
	default:
		err = fmt.Errorf("unknown nineAttacher requested: %v", api)
	}

	return
}
