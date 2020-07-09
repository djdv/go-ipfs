package p9fsp

import (
	"context"
	"strings"
	"time"

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"

	ninelib "github.com/hugelgupf/p9/p9"

	"github.com/ipfs/go-ipfs/filesystem"
)

// bind a `filesystem.Interface` to a 9P Provider
func newAttacher(fs filesystem.Interface, opts ...Option) ninelib.Attacher {
	logName := "/" + strings.ToLower(fs.ID().String())
	settings := parseAttachOptions(maybeAppendLog(opts, logName)...)

	// TODO: read-only option
	// always use cached items if available - otherwise assume data may change between calls

	fid := &fid{
		nodeInterface: fs,
		log:           settings.log,
		initTime:      time.Now(), // TODO: this should be done on `file.Attach()`
	}

	return fid
}

// TODO: options
// we need to supply the log as well
func HostAttacher(ctx context.Context, fs filesystem.Interface) host.Attacher {
	return &nineAttacher{
		ctx:     ctx,
		srv:     ninelib.NewServer(newAttacher(fs)),
		servers: make(map[string]serverRef),
	}
}
