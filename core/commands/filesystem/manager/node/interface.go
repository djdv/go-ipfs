package node

import (
	"context"

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager"
	"github.com/ipfs/go-ipfs/filesystem"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

// TODO: options
//func NewDispatcher(ctx context.Context, core coreiface.CoreAPI, opts ...manager.Option) (Attacher, error) {
func NewDispatcher(ctx context.Context, core coreiface.CoreAPI, opts ...Option) (manager.Dispatcher, error) {
	// TODO: reconsider defaults; we probably want to hostAttach in the foreground by default, not in the background
	settings := parseOptions(maybeAppendLog(opts, filesystem.LogGroup)...)

	return &apiMux{
		ctx:       ctx,
		log:       settings.log,
		NameIndex: manager.NewNameIndex(),
		hosts:     make(hostMap),
		getFS: func(id filesystem.ID) (filesystem.Interface, error) {
			return newFileSystem(ctx, id, core, settings.filesAPIRoot)
		},
	}, nil
}
