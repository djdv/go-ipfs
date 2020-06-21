package p9fsp

import (
	"errors"
	"sync"

	mountinter "github.com/ipfs/go-ipfs/filesystem/mount"
)

var _ mountinter.Instance = (*mountInstance)(nil)

type mountInstance struct {
	providerMu             sync.Locker
	providerDetachCallback func(target string) error
	target                 string
}

func (mi *mountInstance) Detach() error {
	mi.providerMu.Lock()
	defer mi.providerMu.Unlock()

	if err := mountinter.PlatformDetach(mi.target); err != nil {
		return err
	}

	return mi.providerDetachCallback(mi.target)
}

func (mi *mountInstance) Where() (string, error) {
	if mi.target == "" {
		return "", errors.New("instance is not attached")
	}
	return mi.target, nil
}
