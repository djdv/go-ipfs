package fscmds

import (
	"context"
	"fmt"

	"github.com/ipfs/go-ipfs/filesystem"
	"github.com/ipfs/go-ipfs/filesystem/manager"
	"github.com/ipfs/go-ipfs/filesystem/manager/errors"
	"github.com/multiformats/go-multiaddr"
)

type (
	requestHeader struct {
		filesystem.API
		filesystem.ID
	}

	section struct {
		requestHeader
		manager.Requests
	}
	sectionStream = <-chan section
)

// splitRequest returns each component of the request, as an individual typed values.
func splitRequest(request manager.Request) (hostAPI filesystem.API, nodeAPI filesystem.ID, remainder manager.Request, err error) {
	// NOTE: we expect the request to contain a pair of API values as its first component (e.g. `/fuse/ipfs/`)
	// with or without a remainder (e.g. remainder may be `nil`, `.../path/mnt/ipfs/...`, etc.)
	defer func() { // multiaddr pkg will panic if the request is malformed
		if grace := recover(); grace != nil { // so we exorcise the goroutine if this happens
			err = fmt.Errorf("splitRequest panicked: %v - %v", request, grace)
		}
	}()

	var ( // decapsulation
		header                     *multiaddr.Component
		hostProtocol, nodeProtocol int
	)
	header, remainder = multiaddr.SplitFirst(request)
	hostProtocol = header.Protocol().Code
	if nodeProtocol, _, err = multiaddr.ReadVarintCode(header.RawValue()); err != nil {
		return
	}

	// disambiguation
	// Note the direct use of the return variables in the range clauses.
	// If both values being inspected appear in our supported list, we'll return them.
	for _, hostAPI = range []filesystem.API{
		filesystem.Fuse,
	} {
		if hostAPI == filesystem.API(hostProtocol) {
			for _, nodeAPI = range []filesystem.ID{
				filesystem.IPFS,
				filesystem.IPNS,
			} {
				if nodeAPI == filesystem.ID(nodeProtocol) {
					return
				}
			}
		}
	}

	err = fmt.Errorf("unsupported API pair: %v in request %v", header, request)
	return
}

// TODO: still needs a review pass
// splitRequests takes in a stream of requests and returns a channel for each unique request header it finds
func splitRequests(ctx context.Context, requests manager.Requests) (sectionStream, errors.Stream) {
	sections, errors := make(chan section), make(chan error)
	sectionIndex := make(map[requestHeader]chan manager.Request)
	go func() {
		defer close(sections)
		defer close(errors)
		for request := range requests {
			hostAPI, nodeAPI, apiRequest, err := splitRequest(request)
			if err != nil {
				select {
				case errors <- err:
				case <-ctx.Done():
				}
				return // bail in either case
			}

			header := requestHeader{API: hostAPI, ID: nodeAPI}

			// create a stream to send this request on
			// or use an existing one we made before
			requestDestination, exists := sectionIndex[header]
			if !exists {
				requestDestination = make(chan manager.Request, 1)
				sectionIndex[header] = requestDestination

				requestDestination <- apiRequest // buffer the (sub-)request
				select {                         // and block on the section send
				case sections <- section{
					requestHeader: header,
					Requests:      requestDestination,
				}:
				case <-ctx.Done():
					return
				}
			} else { // section is already in the hands of the caller
				select { // block on (sub-)request send
				case requestDestination <- apiRequest:
				case <-ctx.Done():
					return
				}
			}
		}

		for _, sectionStream := range sectionIndex {
			close(sectionStream)
		}
	}()

	return sections, errors
}
