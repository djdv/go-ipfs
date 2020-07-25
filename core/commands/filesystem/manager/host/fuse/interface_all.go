package fuse

import "github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"

// Mounter accepts requests to mount targets on the host FS
// via the FUSE API
type Mounter interface {
	Mount(...Request) <-chan host.Response
}
