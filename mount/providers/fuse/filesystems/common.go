package fusemeta

import (
	"context"
	"sync"

	"github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

type FUSEBase struct {
	//TODO: swap for lock interface
	// index's (lookup) lock
	// RLock should be retained for lookups
	// Lock should be retained when altering index (including cache)
	//sync.RWMutex
	sync.Mutex

	// IPFS api
	Core      coreiface.CoreAPI
	FilesRoot *mfs.Root

	// mount interface
	Ctx        context.Context
	InitSignal chan error
	//parentCtx context.Context
	//mountPoint string
	//mountTime fuse.Timespec
}
