//go:build !nofuse
// +build !nofuse

package fscmds

import (
	"context"
	"sync"
	"time"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs/core"
	"github.com/ipfs/go-ipfs/filesystem/manager"
	"github.com/ipfs/go-ipfs/filesystem/manager/errors"
)

// TODO: inline this?
// TODO: make sure to write clear notes about context lifetimes
// `ipfs daemon` has a `Request.Context` which lives beyond the node's context
func NewDaemonInterface(ctx context.Context, re cmds.ResponseEmitter, node *core.IpfsNode) (fsi manager.Interface, daemonChan errors.Stream, err error) {
	// TODO: take in or construct a formatted emitter (maybe bring back `daemonEmitter`)
	// TODO: return messages to the daemon error channel, during fsi lifetime
	if fsi, err = NewNodeInterface(ctx, node); err == nil {
		errs := make(chan error)
		daemonChan = errs
		go func() {
			<-node.Context().Done()
			// TODO: on daemon shutdown, block until all active instances are closed (or timeout)
			// conditional print; if nothing is mounted, don't print anything
			//`for range fsi.Close(); errs <- resp.Error`
			// for now, we just pretend to do something

			timeout, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			for err := range closeAll(re, fsi.List(timeout)) {
				errs <- err
			}
			close(errs)
		}()
	}
	return
}

// TODO: refactor if we can, try to eliminate wg
// FIXME: ref d12bd32c-bc7a-49d7-ab2d-d161972b924b
// depending on who gets the request first, determines a deadlock or not
// bind needs to background section binds

func (ci *commandDispatcher) Bind(ctx context.Context, requests <-chan manager.Request) <-chan manager.Response {
	sections, errors := generatePipeline(ctx, ci.IpfsNode, requests)
	subResponses := make(chan manager.Responses, len(sections)+1)
	go func() {
		defer close(subResponses)
		var wg sync.WaitGroup
		for sections != nil || errors != nil {
			select {
			case section, ok := <-sections:
				if !ok {
					sections = nil
					continue
				}
				wg.Add(1)

				// figure out which binder should process this section
				header := section.requestHeader
				binder := ci.dispatch[header]

				// binder-requests will not contain our manager-header values,
				// as such - binder-response values will not contain them either.
				// We make sure to restore them before responding to the caller.
				// (e.g. `/fuse/ipfs/path/mnt/ipfs` -> manager -> binder `/path/mnt/ipfs` ->
				// manager `/fuse/ipfs/path/mnt/ipfs` -> ...)

				go func() {
					select {
					case subResponses <- prefixResponses(ctx, header,
						binder.Bind(ctx, section.Requests)):
					case <-ctx.Done():
						return
					}
					wg.Done()
				}()

			case err, ok := <-errors:
				if !ok {
					errors = nil
					continue
				}

				errResp := make(chan manager.Response, 1)
				errResp <- manager.Response{Error: err}

				select {
				case subResponses <- errResp:
				case <-ctx.Done():
					return
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	return handleResponses(ctx, ci.index, mergeResponseStreams(ctx, subResponses))
}
