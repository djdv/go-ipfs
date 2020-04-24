package transform

import (
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

type IOError struct {
	ExternalErr    error
	LocalErrString string
}

// TODO: better error mechanisms; these are very flat and not helpful
var (
	ErrIOReadOnly = IOError{LocalErrString: "write request on read-only system"}
	ErrNotFile    = IOError{LocalErrString: "File request on non-file"}
)

func (e IOError) Error() string {
	if e.LocalErrString != "" {
		return e.LocalErrString
	}
	return e.ExternalErr.Error()
}

// TODO: consider adding an opt parameter; e.g. OpenOP, ReadOP, etc to better conform the errno
func (e IOError) ToFuse() int {
	switch e {
	case ErrIOReadOnly:
		return -fuselib.EROFS
	case ErrNotFile:
		return -fuselib.EISDIR // FIXME: context sensitive
	default:
		return -fuselib.EIO
	}
}

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
