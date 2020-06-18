package fuse

import (
	"errors"
	"sync"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
)

var _ mountinter.Instance = (*mountInstance)(nil)

type mountInstance struct {
	host                   *fuselib.FileSystemHost
	providerMu             sync.Locker
	providerDetachCallback func(target string) error
	target                 string
}

func (mi *mountInstance) Detach() error {
	if !mi.host.Unmount() {
		//TODO: see if we can get better info from the host or something
		return errors.New("failed to unmount")
	}

	return mi.providerDetachCallback(mi.target)
}

func (mi *mountInstance) Where() (string, error) {
	if mi.target == "" {
		return "", errors.New("instance is not attached")
	}
	return mi.target, nil
}
