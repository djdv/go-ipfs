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

	var (
		responses = make(chan Response)
		queue     []host.Request

		hostTasks  sync.WaitGroup
		hostHeader Header
	)

	// call the host API method with a batch of sysID specific requests
	// e.g. Bind(ipfsRequests...), Detach(mfsRequests...)
	dispatchToMethod := func(wg *sync.WaitGroup, header Header, hostRequests ...host.Request) {
		if len(hostRequests) == 0 {
			return // do nothing, don't stall
		}
		wg.Add(1)

		go func() {
			defer wg.Done()
			// TODO: inspect error; if present, call an "unwind" closure
			responses <- Response{
				Header:   header,
				FromHost: hostMethod(header.API, header.ID, hostRequests...),
			}
		}()
	}

	// divide requests into unique lists, based on their Header {API:FS}
	// before we start processing the next group of Headers
	// we'll dispatch the existing group of requests to their method
	requestsEnd := len(requests) - 1
	for i, req := range requests {
		if i == 0 {
			hostHeader = req.Header
		}

		if hostHeader != req.Header { // header changed, new group
			requestBatch := make([]host.Request, len(queue))          // make a group request slice
			copy(requestBatch, queue)                                 // copy the queue into it
			dispatchToMethod(&hostTasks, hostHeader, requestBatch...) // and send it to the host method for processing

			queue = queue[:0]       // re-use the queue slice (to avoid realloc)
			hostHeader = req.Header // set group separator to the new header
		}

		queue = append(queue, req.HostRequest) // append request to host batch queue

		if i == requestsEnd { // if we're the last request, fire off the queue
			dispatchToMethod(&hostTasks, hostHeader, queue...)
		}
	}

	go func() {
		hostTasks.Wait() // wait for all bind requests to respond
		close(responses)
	}()

	return responses
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
