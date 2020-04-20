package transform

import (
	"errors"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	ninelib "github.com/hugelgupf/p9/p9"
	gomfs "github.com/ipfs/go-mfs"
)

type IOFlags uint

const (
	ioNone IOFlags = iota
	IOReadOnly
	IOWriteOnly
	IOReadWrite
	/* consider if we want to support
	   append (writes)
	   create (conditionally)
	   excl(usive create)
	   trunc(ate file)
	*/
)

var (
	ErrIOReadOnly = errors.New("write request on read-only system")
)

func IOFlagsFrom9P(nineFlagsAmusementPark ninelib.OpenFlags) IOFlags {
	switch nineFlagsAmusementPark {
	case ninelib.ReadOnly:
		return IOReadOnly
	case ninelib.WriteOnly:
		return IOWriteOnly
	case ninelib.ReadWrite:
		return IOReadWrite
	default:
		return ioNone
	}
}

func IOFlagsFromFuse(fuseFlags int) IOFlags {
	switch fuseFlags {
	case fuselib.O_RDONLY:
		return IOReadOnly
	case fuselib.O_WRONLY:
		return IOWriteOnly
	case fuselib.O_RDWR:
		return IOReadWrite
	default:
		return ioNone
	}
}

func (f IOFlags) ToMFS() gomfs.Flags {
	switch f {
	case IOReadOnly:
		return gomfs.Flags{Read: true}
	case IOWriteOnly:
		return gomfs.Flags{Write: true}
	case IOReadWrite:
		return gomfs.Flags{Read: true, Write: true}
	default:
		return gomfs.Flags{}
	}
}
