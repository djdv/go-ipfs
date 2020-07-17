package p9fsp

import (
	"context"
	gopath "path"
	"sync"
	"time"

	ninelib "github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"
	"github.com/ipfs/go-ipfs/filesystem"
	logging "github.com/ipfs/go-log"
)

// TODO: how do we do this; without a dependency loop? [6c751cf6-1fb1-4893-8a31-8f9d20b4c38c]
// (regardless of type `manager.const string` or `manager.Stringer`)
//const logGroup = manager.Plan9Protocol
const logGroup = "9P"

type Attacher interface {
	Attach(...Request) <-chan host.Response
}

// manages host bindings with the 9P protocol
type nineAttacher struct {
	sync.Mutex
	log logging.EventLogger

	// 9P transport(s)
	srvCtx  context.Context
	srv     *ninelib.Server      // the actual file instance server that requests are bound to
	servers map[string]serverRef // target <-> server index

	// host node
	//host.PathInstanceIndex // target <-> binding index
}

// bind a `filesystem.Interface` to a host nineAttacher (file system manager format)
func HostAttacher(ctx context.Context, fs filesystem.Interface, opts ...host.Option) (Attacher, error) {
	settings := host.ParseOptions(opts...)
	return &nineAttacher{
		srvCtx: ctx,
		log: logging.Logger(gopath.Join(
			settings.LogPrefix, // fmt: `filesystem`
			logGroup,           // fmt: `9P`
		)),
		srv:     ninelib.NewServer(newAttacher(fs)),
		servers: make(map[string]serverRef),
	}, nil
}

// bind a `filesystem.Interface` to a 9P nineAttacher (9P library format)
func newAttacher(fs filesystem.Interface, opts ...host.Option) ninelib.Attacher {
	settings := host.ParseOptions(opts...)

	fid := &fid{
		nodeInterface: fs,
		log: logging.Logger(gopath.Join(
			settings.LogPrefix, // fmt: `filesystem`
			logGroup,           // fmt: `9P`
			fs.ID().String(),   // fmt: `IPFS`
		)),

		initTime: time.Now(), // TODO: this should be done on `file.Attach()`
	}

	return fid
}
