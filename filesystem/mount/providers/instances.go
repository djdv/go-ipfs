package providercommon

import (
	"errors"
	"fmt"
	"io"

	"github.com/ipfs/go-ipfs/filesystem/mount"
)

// Instance is a `Request` that has been instantiated by a provider and is indexed by its `.Target`.
type Instance interface {
	List() mount.Request // the `Request` that created this `Instance`
	io.Closer            // the mechanism to decouple the `Instance` from the `Request`
}

type (
	InstanceStack interface {
		Push(request mount.Request, fsInstance io.Closer)
		Clear()
		Unwind() error
		Length() int

		// XXX: nasty C-ism that lets us do witchcraft here but prevents anyone from implementing this
		// TODO: move to /internal maybe but keep this method private regardless
		expose() ([]mount.Request, []io.Closer)
	}

	instanceStack struct {
		requests        []mount.Request
		instanceClosers []io.Closer
	}

	stackWrapper struct {
		mount.Request
		io.Closer
	}
)

func (sw *stackWrapper) List() mount.Request { return sw.Request }

func (is *instanceStack) expose() ([]mount.Request, []io.Closer) {
	return is.requests, is.instanceClosers
}
func (is *instanceStack) Clear()      { is.requests = nil; is.instanceClosers = nil }
func (is *instanceStack) Length() int { return len(is.requests) }
func (is *instanceStack) Push(req mount.Request, fsInstance io.Closer) {
	is.requests = append(is.requests, req)
	is.instanceClosers = append(is.instanceClosers, fsInstance)
}

func errWrap(target string, base, appended error) error {
	if base == nil {
		base = fmt.Errorf("%q:%s", target, appended)
	} else {
		base = fmt.Errorf("%w; %q:%s", base, target, appended)
	}
	return base
}

func (is *instanceStack) Unwind() error {
	var err error
	for i := len(is.instanceClosers) - 1; i != -1; i-- {
		instance := is.instanceClosers[i]
		if iErr := instance.Close(); iErr != nil {
			err = errWrap(is.requests[i].Target, err, iErr)
		}
	}
	return err
}

func NewInstanceStack(allocHint int) InstanceStack {
	return &instanceStack{
		requests:        make([]mount.Request, 0, allocHint),
		instanceClosers: make([]io.Closer, 0, allocHint),
	}
}

type (
	InstanceCollection interface {
		Add(InstanceStack)
		Exists(target string) bool
		List() []mount.Request
		Detach(...mount.Request) error
		io.Closer // closes all
	}

	// TODO: either make this thread safe or add an explicit note about it
	// for now it makes more sense to assume the provider using it is locked
	// maybe best to internalize all commons/utils packages too
	instanceMap map[string]Instance
)

func NewInstanceCollection() InstanceCollection {
	return make(instanceMap)
}

// TODO: document the why behind this, and/or refactor it
// we want providers to have Close/CloseSpecific access on demand
// but adds must go through a stack instance; so that `Bind` calls are transactional
// they either all succeed and get added, or on error the caller calls the stack Unwind
// also note that duplicate checking is the responsibility of the caller
// (we'll overwrite instances of the same target here, so they should call Exists themselves)
// we also implement `List` so the caller doesn't have to
func (im instanceMap) Add(stack InstanceStack) {
	reqs, closers := stack.expose()
	for i, req := range reqs {
		im[req.Target] = &stackWrapper{Request: req, Closer: closers[i]}
	}
}

func (im instanceMap) Exists(target string) bool { _, ok := im[target]; return ok }

func (im instanceMap) List() []mount.Request {
	ret := make([]mount.Request, 0, len(im))
	for _, instance := range im {
		ret = append(ret, instance.List())
	}
	return ret
}

// Close specific references, and stop tracking them
func (im instanceMap) Detach(requests ...mount.Request) error {
	var err error
	for _, req := range requests {
		target := req.Target
		instance, ok := im[target]
		if !ok {
			err = errWrap(target, err, errors.New("not bound"))
			continue
		}

		instanceErr := instance.Close()
		if instanceErr != nil {
			err = errWrap(target, err, instanceErr)
		}
		// regardless of the error, stop tracking this reference
		delete(im, req.Target)
	}
	return err
}

// Close all references
func (im instanceMap) Close() error { return im.Detach(im.List()...) }
