package providercommon

import (
	"fmt"

	mountinter "github.com/ipfs/go-ipfs/filesystem/mount"
)

type mountPoint = string

type (
	instanceMap        map[mountPoint]mountinter.Instance
	InstanceCollection interface {
		Add(target string, mi mountinter.Instance) error
		Exists(target string) bool
		Get(target string) (mountinter.Instance, error)
		Remove(target string) error
		Length() int
		List() []string
	}
)

func NewInstanceCollection() instanceMap {
	return make(instanceMap)
}

func (im instanceMap) Exists(target string) bool {
	_, ok := im[target]
	return ok
}

func (im instanceMap) Add(target string, mi mountinter.Instance) error {
	if im.Exists(target) {
		return fmt.Errorf("%q already bound", target)
	}

	im[target] = mi
	return nil
}

func (im instanceMap) Remove(target string) error {
	if !im.Exists(target) {
		return fmt.Errorf("%q was not bound", target)
	}

	delete(im, target)
	return nil
}

func (im instanceMap) Get(target string) (mountinter.Instance, error) {
	if instance, ok := im[target]; ok {
		return instance, nil
	}
	return nil, fmt.Errorf("%q was not attached", target)
}

func (im instanceMap) Length() int {
	return len(im)
}

func (im instanceMap) List() []string {
	ret := make([]string, im.Length())
	for target := range im {
		ret = append(ret, target)
	}
	return ret
}

type (
	instanceStateMap        map[mountPoint]struct{}
	InstanceCollectionState interface {
		Add(target string) error
		Exists(target string) bool
		Remove(target string) error
		Length() int
		List() []string
	}
)

// TODO: see if we can avoid this dumb thing (quick hack/port)
// what we want is for the conductor to have its instance map
// then pass that map to some transform  which forbids access to the instances `stateFromMap(map)(stateInterface)`
// but gives access to its `ok` state to check if things exists and insert targets arbitrarily
// sharing this without making the API/constructors complicated may not be doable though
func NewInstanceCollectionState() instanceStateMap {
	return make(instanceStateMap)
}

func (im instanceStateMap) Exists(target string) bool {
	_, ok := im[target]
	return ok
}

func (im instanceStateMap) Add(target string) error {
	if im.Exists(target) {
		return fmt.Errorf("%q already bound", target)
	}

	im[target] = struct{}{}
	return nil
}

func (im instanceStateMap) Remove(target string) error {
	if !im.Exists(target) {
		return fmt.Errorf("%q was not bound", target)
	}

	delete(im, target)
	return nil
}

func (im instanceStateMap) Length() int {
	return len(im)
}

func (im instanceStateMap) List() []string {
	ret := make([]string, 0, im.Length())
	for target := range im {
		ret = append(ret, target)
	}
	return ret
}
