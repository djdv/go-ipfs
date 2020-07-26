package manager

import (
	"fmt"
	"sort"
	"sync"

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"
	"github.com/ipfs/go-ipfs/filesystem"
)

type byTarget []Request

func (r byTarget) Len() int      { return len(r) }
func (r byTarget) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r byTarget) Less(i, j int) bool {
	return r[i].HostRequest.String() < r[j].HostRequest.String()
}

type byAPI []Request

func (r byAPI) Len() int      { return len(r) }
func (r byAPI) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r byAPI) Less(i, j int) bool {
	return r[i].API < r[j].API &&
		r[i].ID < r[j].ID &&
		r[i].HostRequest.String() < r[j].HostRequest.String()
}

func check(ni NameIndex, requests ...Request) error {
	// basic dupe check
	// sort by target string
	if len(requests) < 2 {
		return nil
	}

	sort.Sort(byTarget(requests))

	rightShifted := requests[1:]
	for i, rightRequest := range rightShifted {
		if requests[i].String() == rightRequest.String() {
			return fmt.Errorf("duplicate target requested: %q", rightRequest.String())
		}

		// if already in the index, deny request
		if ni.Exist(rightRequest) {
			return fmt.Errorf("%q is already bound", rightRequest.String())
		}
	}

	return nil
}

type hostMethod func(api API, id filesystem.ID, requests ...host.Request) <-chan host.Response

func dispatch(hostMethod hostMethod, requests ...Request) <-chan Response {
	sort.Sort(byAPI(requests))

	merged := make(chan Response)

	dispatchToMethod := func(wg *sync.WaitGroup, header Header, hostRequests ...host.Request) {
		if len(hostRequests) == 0 {
			return
		}
		wg.Add(1)

		go func() {
			defer wg.Done()
			// TODO: inspect error
			// if present, call an "unwind" closure
			merged <- Response{
				Header:   header,
				FromHost: hostMethod(header.API, header.ID, hostRequests...),
			}
		}()
	}

	var hostTasks sync.WaitGroup
	var hostHeader Header
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

		requestBatch = append(requestBatch, req.HostRequest)

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

func (am *apiMux) splitRequests(hostMethod hostMethod, requests ...Request) <-chan Response {
	if err := check(am.NameIndex, requests...); err != nil {

		// create the return channels with a message buffer
		hostResp := make(chan host.Response, 1)
		managerResp := make(chan Response, 1)

		hostResp <- host.Response{Error: err}       // stuff the error into the channel buffer
		managerResp <- Response{FromHost: hostResp} // and send it through the manager's response channel

		close(hostResp)
		close(managerResp)

		return managerResp // return the buffered message(s)
	}
	return dispatch(hostMethod, requests...)
}
