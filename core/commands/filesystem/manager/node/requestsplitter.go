package node

import (
	"fmt"
	"sort"
	"sync"

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"
	"github.com/ipfs/go-ipfs/filesystem"
)

type byTarget []manager.Request

func (r byTarget) Len() int      { return len(r) }
func (r byTarget) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r byTarget) Less(i, j int) bool {
	return r[i].Target < r[j].Target
}

type byAPI []manager.Request

func (r byAPI) Len() int      { return len(r) }
func (r byAPI) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r byAPI) Less(i, j int) bool {
	return r[i].API < r[j].API &&
		r[i].ID < r[j].ID &&
		r[i].Target < r[j].Target
}

func sideBySide(left, right manager.Request) error {
	leftIndex := manager.RequestIndex(left)
	if leftIndex == manager.RequestIndex(right) {
		return fmt.Errorf("duplicate target requested: %q", leftIndex)
	}
	return nil
}

func check(ni manager.NameIndex, requests ...manager.Request) error {
	// basic dupe check
	// sort by target string
	if len(requests) < 2 {
		return nil
	}

	sort.Sort(byTarget(requests))

	rightShifted := requests[1:]
	for i, rightRequest := range rightShifted {
		if err := sideBySide(requests[i], rightRequest); err != nil {
			return err
		}

		// if already in the index, deny request
		if ni.Exist(rightRequest) {
			return fmt.Errorf("%q is already bound", rightRequest.Target)
		}
	}

	return nil
}

type hostMethod func(api manager.API, sysID filesystem.ID, requests ...host.Request) <-chan host.Response

func dispatch(hostMethod hostMethod, requests ...manager.Request) <-chan manager.Response {
	sort.Sort(byAPI(requests))

	merged := make(chan manager.Response)

	dispatchToMethod := func(wg *sync.WaitGroup, header manager.Header, hostRequests ...host.Request) {
		if len(hostRequests) == 0 {
			return
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			// TODO: inspect error
			// if present, call an "unwind" closure
			merged <- manager.Response{
				Header:   header,
				FromHost: hostMethod(header.API, header.ID, hostRequests...),
			}
		}()
	}

	var hostTasks sync.WaitGroup
	var hostHeader manager.Header
	var requestBatch []host.Request

	// build a unique list for each pair of {API:FS} requests
	// dispatching the request as soon as all elements are prepared
	rEnd := len(requests) - 1
	for i, req := range requests {
		if i == 0 {
			hostHeader = req.Header
		}

		if hostHeader != req.Header {
			hostRequests := make([]host.Request, len(requestBatch))
			copy(hostRequests, requestBatch) // copy contents into a new array
			// send them off to their handler
			dispatchToMethod(&hostTasks, hostHeader, hostRequests...)

			requestBatch = requestBatch[:0] // re-use the buffer slice
			hostHeader = req.Header         // move batch marker forward
		}

		requestBatch = append(requestBatch, req.Request)

		if i == rEnd {
			dispatchToMethod(&hostTasks, hostHeader, requestBatch...)
		}
	}

	go func() {
		hostTasks.Wait()
		close(merged)
	}()

	return merged
}

func (am *apiMux) splitRequests(hostMethod hostMethod, requests ...manager.Request) <-chan manager.Response {
	if err := check(am.NameIndex, requests...); err != nil {
		hostResp := make(chan host.Response, 1)
		hostResp <- host.Response{Error: err}
		close(hostResp)

		resp := make(chan manager.Response, 1)
		resp <- manager.Response{FromHost: hostResp}
		close(resp)

		return resp
	}
	return dispatch(hostMethod, requests...)
}
