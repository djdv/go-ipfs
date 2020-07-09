package node

import (
	"context"
	"fmt"
	"sync"

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"
	p9fsp "github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/9p"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/fuse"
	"github.com/ipfs/go-ipfs/filesystem"
	"github.com/ipfs/go-ipfs/filesystem/interface/ipfscore"
	"github.com/ipfs/go-ipfs/filesystem/interface/keyfs"
	"github.com/ipfs/go-ipfs/filesystem/interface/mfs"
	"github.com/ipfs/go-ipfs/filesystem/interface/pinfs"
	logging "github.com/ipfs/go-log"
	gomfs "github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

type hostMap map[manager.API]map[filesystem.ID]host.Attacher

// apiMux handles requests for several host APIs
type apiMux struct {
	sync.Mutex
	ctx context.Context
	log logging.EventLogger

	manager.NameIndex

	hosts hostMap // {API:ID} host manager
	getFS func(filesystem.ID) (filesystem.Interface, error)
}

func newFileSystem(ctx context.Context, sysID filesystem.ID, core coreiface.CoreAPI, filesAPIRoot *gomfs.Root) (fs filesystem.Interface, err error) {
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
		err = fmt.Errorf("unknown Attacher requested: %v", sysID)
	}

	return
}

// TODO: do unwind here
// host hostAttach should just stop on error (no unwind)
func (am *apiMux) Attach(requests ...manager.Request) <-chan manager.Response {
	return am.splitRequests(am.hostAttach, requests...)
}

func (am *apiMux) hostAttach(api manager.API, sysID filesystem.ID, requests ...host.Request) <-chan host.Response {
	am.Lock()
	defer am.Unlock()

	responses := make(chan host.Response, 1)

	hostAttacher, err := am.getHostAttacher(api, sysID)
	if err != nil {
		responses <- host.Response{Error: err}
		close(responses)
		return responses
	}

	go func() {
		for resp := range hostAttacher.Attach(requests...) {
			responses <- resp
			if resp.Error != nil {
				// TODO: unwind stack and exit
				continue
			}

			am.NameIndex.Push(manager.IndexValue{
				Header:  manager.Header{API: api, ID: sysID},
				Binding: resp.Binding,
			})
		}

		am.NameIndex.Commit()
		close(responses)
	}()

	return responses
}

func (am *apiMux) getHostAttacher(api manager.API, sysID filesystem.ID) (attacher host.Attacher, err error) {
	hosts, ok := am.hosts[api]
	if !ok {
		hosts := make(map[filesystem.ID]host.Attacher)
		am.hosts[api] = hosts
	}

	attacher, ok = hosts[sysID]
	if ok {
		return
	}

	var fs filesystem.Interface
	fs, err = am.getFS(sysID)
	if err != nil {
		return
	}

	switch api {
	default:
		err = fmt.Errorf("unknown provider: %q", api)
	case manager.Plan9Protocol:
		attacher = p9fsp.HostAttacher(am.ctx, fs)
	case manager.Fuse:
		attacher = fuse.HostAttacher(am.ctx, fs)
	}
	if err != nil {
		hosts[sysID] = attacher
	}

	return
}
