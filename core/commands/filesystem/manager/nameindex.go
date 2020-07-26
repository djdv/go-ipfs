package manager

import (
	"sync"

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"
	"github.com/ipfs/go-ipfs/filesystem"
)

type indexValue struct {
	Header
	host.Binding
}

type NameIndex interface {
	Exist(Request) bool
	Push(indexValue)
	Commit()

	Detach(...Request) <-chan Response
	List() <-chan Response
	//Yield() []host.HostRequest
}

// TODO: better tree structure
type instanceIndex map[API]systemIndex
type systemIndex map[filesystem.ID]targetIndex
type targetIndex map[string]host.Binding

type nameIndex struct {
	sync.Mutex

	// we store request responses in a queue
	// if something goes wrong during operation, we may unwind the queue
	// to attempt to undo the request transaction
	// if the entire operation succeeds
	// we can commit the results to an index
	stack     []indexValue
	instances instanceIndex
}

func NewNameIndex() NameIndex {
	return &nameIndex{
		instances: make(instanceIndex),
	}
}

// TODO: push, commit, unwind
// unwind must return instance.Detach channel
// takes in a closure?
// stack.Unwind(unwFunc) <-chan outStream {
// for each stack elem { unwFunc(elem) }
//
// commit flushes the stack into the index

func (ni *nameIndex) Push(result indexValue) {
	ni.Lock()
	defer ni.Unlock()
	ni.stack = append(ni.stack, result)
}

/*
func (ni *nameIndex) Yield() []host.HostRequest {
	ni.Lock()
	defer ni.Unlock()
	reqs := make([]host.HostRequest, 0, len(ni.stack))
	for i := len(ni.stack) - 1; i != -1; i-- { // return in reverse order
		reqs = append(reqs, ni.stack[i].HostRequest)
	}
	ni.stack = ni.stack[:0]
	return reqs
}
*/

//type detachFunc func(...instance.requests) <-chan instance.outStream

/*
func (ni *nameIndex) Unwind(detach DetachFunc) <-chan instance.outStream {
	ni.Lock()
	defer ni.Unlock()
	reqs := make([]instance.requests, 0, len(ni.stack))
	for _, resp := range ni.stack {
		reqs = append(reqs, resp.requests)
	}

	ni.stack = ni.stack[:0]
	return detach(reqs...)
}
*/

type closer func() error      // io.Closure closure wrapper
func (f closer) Close() error { return f() }

func (ni *nameIndex) Commit() {
	ni.Lock()
	defer ni.Unlock()

	for _, index := range ni.stack { // flush the stack to the target index
		// walk the index, creating as needed
		api := index.API
		systems, ok := ni.instances[api]
		if !ok {
			systems = make(systemIndex)
			ni.instances[api] = systems
		}

		sysID := index.ID
		targets, ok := systems[sysID]
		if !ok {
			targets = make(targetIndex)
			systems[sysID] = targets
		}

		// insert into index and remove self when closed
		indexName := index.Binding.String()

		bindCloser := index.Binding.Closer.Close
		index.Binding.Closer = closer(func() error {
			ni.Lock()
			defer ni.Unlock()
			delete(targets, indexName)
			if len(targets) == 0 {
				delete(systems, sysID)
			}
			if len(systems) == 0 {
				delete(ni.instances, api)
			}

			return bindCloser()
		})

		targets[indexName] = index.Binding
	}

	ni.stack = ni.stack[:0] // clear the stack
}

func (ni *nameIndex) Exist(request Request) bool {
	ni.Lock()
	defer ni.Unlock()

	return ni.unpackRequestTarget(request) != ""
}

func (ni *nameIndex) unpackRequestTarget(request Request) (indexName string) {
	for api, systems := range ni.instances {
		if api == request.API && systems != nil {
			if system, fsInUse := systems[request.ID]; fsInUse {
				if binding, indexInUse := system[request.String()]; indexInUse {
					indexName = binding.String()
					return
				}
			}
		}
	}
	return
}

func (ni *nameIndex) List() <-chan Response {
	ni.Lock()
	defer ni.Unlock()

	resp := make(chan Response)

	go func() {
		for api, systems := range ni.instances {
			for sysID, targets := range systems {
				header := Header{API: api, ID: sysID}

				binderChan := make(chan host.Response)

				hostResp := Response{
					Header:   header,
					FromHost: binderChan,
				}

				go func(targets targetIndex) {
					for _, binding := range targets {
						binderChan <- host.Response{Binding: binding}
					}
					close(binderChan)
				}(targets)

				resp <- hostResp
			}
		}
		close(resp)
	}()

	return resp
}

func (ni *nameIndex) Detach(requests ...Request) <-chan Response {
	ni.Lock()
	defer ni.Unlock()

	responses := make(chan Response)
	hostMap := make(map[Header]chan host.Response)

	// TODO: [lazy] a better structure, communication, and traversal for this
	// for each api:system => for each request => close if match
	go func() {
		for api, systems := range ni.instances {
			for sysID, targets := range systems {
				for _, index := range requests {
					if index.API == api &&
						index.ID == sysID {
						if binding, ok := targets[index.HostRequest.String()]; ok {
							// re-use the same channel if it exists for this index's header pair
							hostChan, ok := hostMap[index.Header]
							if !ok {
								hostChan = make(chan host.Response)
								hostMap[index.Header] = hostChan
								// let the receiver know there's a new response channel for `Header`
								defer close(hostChan)
								responses <- Response{
									Header:   index.Header,
									FromHost: hostChan,
								}
							}

							hostChan <- host.Response{
								Binding: binding,
								Error:   binding.Close(),
							}
						}
					}
				}
			}
		}
		close(responses)
	}()

	return responses
}
