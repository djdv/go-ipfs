package manager

import (
	"sync"

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"
	p9fsp "github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/9p"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/fuse"
	"github.com/ipfs/go-ipfs/filesystem"
)

type IndexValue struct {
	Header
	host.Binding
}

type NameIndex interface {
	Exist(Request) bool
	Push(IndexValue)
	Commit()

	Detach(...Request) <-chan Response
	List() <-chan Response
	//Yield() []host.Request
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
	stack     []IndexValue
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

func (ni *nameIndex) Push(result IndexValue) {
	ni.Lock()
	defer ni.Unlock()
	ni.stack = append(ni.stack, result)
}

/*
func (ni *nameIndex) Yield() []host.Request {
	ni.Lock()
	defer ni.Unlock()
	reqs := make([]host.Request, 0, len(ni.stack))
	for i := len(ni.stack) - 1; i != -1; i-- { // return in reverse order
		reqs = append(reqs, ni.stack[i].Request)
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

func (ni *nameIndex) Commit() {
	ni.Lock()
	defer ni.Unlock()

	for _, binding := range ni.stack {
		sysIndex, ok := ni.instances[binding.API]
		if !ok {
			sysIndex = make(systemIndex)
			ni.instances[binding.API] = sysIndex
		}

		binder, ok := sysIndex[binding.ID]
		if !ok {
			binder = make(targetIndex)
			sysIndex[binding.ID] = binder
		}

		var indexName string
		switch binding.API {
		case Plan9Protocol:
			var err error
			indexName, _, _, err = p9fsp.ParseRequest(binding.Request)
			if err != nil {
				panic(err) // the request dispatcher pushed a bad response
			}
		case Fuse:
			_, indexName = fuse.ParseRequest(binding.Request)
		default:
			indexName = binding.Target
		}

		binder[indexName] = binding.Binding
	}

	ni.stack = ni.stack[:0]
}

func (ni *nameIndex) Exist(request Request) bool {
	ni.Lock()
	defer ni.Unlock()

	for api, systems := range ni.instances {
		if api == request.API && systems != nil {
			if system, fsInUse := systems[request.ID]; fsInUse {
				index := RequestIndex(request)
				_, targetInUse := system[index]
				return targetInUse
			}
		}
	}

	return false
}

func (ni *nameIndex) List() <-chan Response {
	ni.Lock()
	defer ni.Unlock()

	resp := make(chan Response)

	go func() {
		for api, systems := range ni.instances {
			for sysID, targetIndex := range systems {
				header := Header{API: api, ID: sysID}

				binderChan := make(chan host.Response)

				hostResp := Response{
					Header:   header,
					FromHost: binderChan,
				}

				go func() {
					for _, binding := range targetIndex {
						binderChan <- host.Response{Binding: binding}
					}
					close(binderChan)
				}()

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

	// TODO: [lazy] a better structure and traversal for this
	// for each api:system => for each request => close if match
	go func() {
		for api, systems := range ni.instances {
			for sysID, targets := range systems {
				for _, request := range requests {
					if request.API == api &&
						request.ID == sysID {
						index := RequestIndex(request)
						if binding, ok := targets[index]; ok {
							hostChan, ok := hostMap[request.Header]
							if !ok {
								hostChan = make(chan host.Response)
								defer close(hostChan)

								hostMap[request.Header] = hostChan
								responses <- Response{
									Header:   request.Header,
									FromHost: hostChan,
								}
							}

							delete(targets, request.Target)
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
