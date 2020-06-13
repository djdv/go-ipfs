package core_test

import (
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/core"
)

var _ fuselib.FileSystemInterface = (*core.FileSystem)(nil)
