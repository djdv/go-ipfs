package pinfs

import transform "github.com/ipfs/go-ipfs/filesystem"

// TODO: error on root
func (pi *pinInterface) Open(path string, flags transform.IOFlags) (transform.File, error) {
	return pi.ipfs.Open(path, flags)
}
