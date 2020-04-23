package providercommon

import (
	"context"
	"runtime"
	"sync"

	mountcom "github.com/ipfs/go-ipfs/mount/utils/common"
	gomfs "github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

// NOTE: [b7952c54-1614-45ea-a042-7cfae90c5361] cgofuse only supports ReaddirPlus on Windows
// if this ever changes (bumps libfuse from 2.8 -> 3.X+), add platform support here (and to any other tags with this UUID)
// TODO: this would be best in the fuselib itself; make a patch upstream
// it's only here because we can't put it in fusecommon because of a dependency cycle
var CanReaddirPlus bool = runtime.GOOS == "windows"

type (
	Base interface {
		sync.Locker           // File system's own lock
		mountcom.ResourceLock // Lock for the resources within the system
		Ctx() context.Context // Context used for operations, when canceled the system should halt and close
		// TODO:	Close() separate from context cancel?
	}

	IPFSCore interface {
		Base
		Core() coreiface.CoreAPI
	}

	MFS interface {
		Base
		Root() *gomfs.Root
	}
)

type (
	base struct {
		sync.Mutex
		mountcom.ResourceLock
		ctx context.Context
	}

	ipfsCore struct {
		*base
		core coreiface.CoreAPI
	}

	mfs struct {
		*base
		root *gomfs.Root
	}

	fuseOverlay struct {
		*ipfsCore
		*mfs
	}
)

func NewBase(ctx context.Context, rl mountcom.ResourceLock) *base {
	return &base{
		ctx:          ctx,
		ResourceLock: rl,
	}
}

func NewIPFSCore(ctx context.Context, core coreiface.CoreAPI, rl mountcom.ResourceLock) *ipfsCore {
	return &ipfsCore{
		base: NewBase(ctx, rl),
		core: core,
	}
}

func NewMFS(ctx context.Context, mroot *gomfs.Root, rl mountcom.ResourceLock) *mfs {
	return &mfs{
		base: NewBase(ctx, rl),
		root: mroot,
	}
}

func (fb *base) Ctx() context.Context        { return fb.ctx }
func (fi *ipfsCore) Core() coreiface.CoreAPI { return fi.core }
func (fm *mfs) Root() *gomfs.Root            { return fm.root }
