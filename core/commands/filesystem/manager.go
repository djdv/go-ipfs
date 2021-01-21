//go:build !nofuse
// +build !nofuse

package fscmds

import (
	"context"
	"sync"

	"github.com/ipfs/go-ipfs/filesystem/manager"
)

// TODO: refactor if we can, try to eliminate wg
// TODO: sync needs review; ^possible send on close for subResponses, etc.
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
