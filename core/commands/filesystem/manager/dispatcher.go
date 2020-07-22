package manager

import (
	"context"
	"sync"

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"
	p9fsp "github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/9p"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/fuse"
	"github.com/ipfs/go-ipfs/filesystem"
	logging "github.com/ipfs/go-log"
)

// apiMux handles requests for several host APIs
type apiMux struct {
	sync.Mutex
	ctx context.Context
	log logging.EventLogger

	//TODO: we need to hold the exit channel
	// when dispatching the last target for something
	// close it
	// return the error up to the manager
	// the manager will relay the error to either the unmount command (normal use)
	// or the daemon (interrupt signal was caught on the daemon and we're closing)

	NameIndex

	getFS           func(filesystem.ID) (filesystem.Interface, error)
	getHostAttacher func(API, filesystem.Interface) (interface{}, error)
}

// TODO: do unwind here
// host hostAttach should just stop on error (no unwind)
func (am *apiMux) Attach(requests ...Request) <-chan Response {
	return am.splitRequests(am.hostAttach, requests...)
}

// TODO: needs both ID and FS (needs ID and getFS(id))
func (am *apiMux) hostAttach(api API, id filesystem.ID, requests ...host.Request) <-chan host.Response {
	am.Lock()
	defer am.Unlock()

	responses := make(chan host.Response, 1)

	fs, err := am.getFS(id)
	if err != nil {
		responses <- host.Response{Error: err}
		close(responses)
		return responses
	}

	hostAttacher, err := am.getHostAttacher(api, fs)
	if err != nil {
		responses <- host.Response{Error: err}
		close(responses)
		return responses
	}

	// XXX: defeating the type system hacks
	proxy := make(chan host.Response, 1)
	go func() {
		var hostChan <-chan host.Response
		switch api {
		case Plan9Protocol:
			typedReqs := make([]p9fsp.Request, len(requests))
			for i := range requests {
				typedReqs[i] = requests[i].(p9fsp.Request)
			}
			hostChan = hostAttacher.(p9fsp.Attacher).Attach(typedReqs...)
		case Fuse:
			typedReqs := make([]fuse.Request, len(requests))
			for i := range requests {
				typedReqs[i] = requests[i].(fuse.Request)
			}
			hostChan = hostAttacher.(fuse.Mounter).Mount(typedReqs...)

		default:
			panic("unexpected API type requested")
		}

		for resp := range hostChan {
			proxy <- resp
		}

		close(proxy)
	}()

	go func() {
		for msg := range proxy {
			responses <- msg

			if msg.Error != nil {
				continue
			}

			am.NameIndex.Push(indexValue{
				Header:  Header{API: api, ID: id},
				Binding: msg.Binding,
			})
		}

		am.NameIndex.Commit()
		close(responses)
	}()

	return responses
}
