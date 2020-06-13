package pinfs

import "github.com/ipfs/go-ipfs/mount/utils/transform"

// TODO: error on root
func (pi *pinInterface) Open(path string, flags transform.IOFlags) (transform.File, error) {
	return pi.ipfs.Open(path, flags)
}
