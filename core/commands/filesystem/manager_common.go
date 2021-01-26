package fscmds

import (
	"context"
	"fmt"
	"sync"

	"github.com/ipfs/go-ipfs/core"
	"github.com/ipfs/go-ipfs/filesystem"
	"github.com/ipfs/go-ipfs/filesystem/manager"
	"github.com/multiformats/go-multiaddr"
)

// TODO: move this or export it; duplicated across pkgs currently
var errUnwound = fmt.Errorf("binding undone")

type (
	dispatchMap map[requestHeader]manager.Binder

	// commandDispatcher manages requests for/from `go-ipfs-cmds`.
	// Dispatching requests to one of several multiplexed binders.
	commandDispatcher struct {
		*core.IpfsNode
		dispatch dispatchMap
		index
	}
)

type (
	indexKey = string
	index    interface {
		fetch(key indexKey) *manager.Response
		store(key indexKey, value *manager.Response)
		List(ctx context.Context) <-chan manager.Response
	}
	indices map[indexKey]*manager.Response
)

func newIndex() index                                          { return make(indices) }
func (ci indices) fetch(key indexKey) *manager.Response        { return ci[key] }
func (ci indices) store(key indexKey, value *manager.Response) { ci[key] = value }
func (ci indices) List(ctx context.Context) <-chan manager.Response {
	respChan := make(chan manager.Response)
	go func() {
		defer close(respChan)
		for _, resp := range ci {
			select {
			case respChan <- *resp:
			case <-ctx.Done():
				return
			}
		}
	}()
	return respChan
}

func prefixResponses(ctx context.Context, header requestHeader, responses manager.Responses) manager.Responses {
	respChan := make(chan manager.Response)
	base, _ := multiaddr.NewComponent(header.API.String(), header.ID.String())
	go func() {
		defer close(respChan)
		for response := range responses {
			if response.Request != nil {
				response.Request = base.Encapsulate(response.Request)
			} else {
				response.Request = base
			}
			select {
			case respChan <- response:
			case <-ctx.Done():
				return
			}
		}
	}()
	return respChan
}

func mergeResponseStreams(ctx context.Context, responseStreams <-chan manager.Responses) manager.Responses {
	var wg sync.WaitGroup
	mergedStream := make(chan manager.Response)
	mergeFrom := func(responses manager.Responses) {
		defer wg.Done()
		for response := range responses {
			select {
			case mergedStream <- response:
			case <-ctx.Done():
				return
			}
		}
	}
	go func() {
		for responses := range responseStreams {
			wg.Add(1)
			mergeFrom(responses)
		}
		go func() { wg.Wait(); close(mergedStream) }()
	}()
	return mergedStream
}

// TODO: English and 3AM logic
// convoluted names and flow mixed with lisp-isms needs a review pass at least
func handleResponses(ctx context.Context, index index, responses <-chan manager.Response) <-chan manager.Response {
	var (
		succeeded    []manager.Response
		respChan     = make(chan manager.Response)
		haveObserver = true // TODO: remark about processing loop being unstoppable / untied to caller's context
		// TODO: obviate these please, this is circus tier
		processResponses = commitResponsesTo(index)
	)
	go func() {
		defer close(respChan)
		for response := range responses { // get all responses
			if response.Error == nil {
				succeeded = append(succeeded, response)
			} else {
				processResponses = closeResponses
			}
			if haveObserver { // regardless of error,
				select { // relay status to observer (if there is one)
				case respChan <- response:
				case <-ctx.Done():
					haveObserver = false
				}
			}
		}

		// TODO: this "maybeReply" needs to be summed up better
		for response := range processResponses(succeeded) {
			if haveObserver {
				select {
				case respChan <- response:
				case <-ctx.Done():
					haveObserver = false
				}
			}
		}
	}()

	return respChan
}

// TODO: quick hacks, needs review
type responseHandlerFunc func([]manager.Response) manager.Responses

func commitResponsesTo(index index) responseHandlerFunc {
	return func(responses []manager.Response) manager.Responses {
		noResponse := make(chan manager.Response)
		close(noResponse)
		for i := range responses {
			instance := responses[i]
			// TODO: we need a standard `request=>indexKey` hashing function
			// anything to prevent duplicate entries
			key, _ := instance.Request.ValueForProtocol(int(filesystem.PathProtocol))
			index.store(key, &instance)
		}
		return noResponse
	}
}

func closeResponses(responses []manager.Response) manager.Responses {
	undoneStatusMessages := make(chan manager.Response, len(responses))
	for i := len(responses) - 1; i != -1; i-- {
		instance := responses[i]
		if cErr := instance.Close(); cErr == nil {
			instance.Error = errUnwound
		} else {
			instance.Error = fmt.Errorf("%w - failed to close: %s", errUnwound, cErr)
		}
		undoneStatusMessages <- instance
	}
	return undoneStatusMessages
}
