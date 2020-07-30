package p9fsp

import (
	"context"
	gopath "path"
	"strings"
	"sync"
	"time"

	ninelib "github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/options"
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
func NewAttacher(ctx context.Context, fs filesystem.Interface, opts ...options.Option) (Attacher, error) {
	settings := options.Parse(opts...)

	return &nineAttacher{
		srvCtx: ctx,
		log: logging.Logger(gopath.Join(
			settings.LogPrefix,        // (opt)fmt: `filesystem`
			strings.ToLower(logGroup), // fmt: `9p`
		)),
		srv:     ninelib.NewServer(newAttacher(fs, opts...)),
		servers: make(map[string]serverRef),
	}, nil
}

// bind a `filesystem.Interface` to a 9P nineAttacher (9P library format)
func newAttacher(fs filesystem.Interface, opts ...options.Option) ninelib.Attacher {
	settings := options.Parse(opts...)

	fid := &fid{
		nodeInterface: fs,
		log: logging.Logger(gopath.Join(
			settings.LogPrefix,                // (opt)fmt: `filesystem`
			strings.ToLower(logGroup),         // fmt: `9p`
			strings.ToLower(fs.ID().String()), // fmt: `ipfs`
		)),

		initTime: time.Now(), // TODO: this should be done on `file.Attach()`
	}

	switch fs.ID() {
	case filesystem.KeyFS, filesystem.Files:
		fid.filesWritable = true
	}

	return fid
}
