package mfs

import (
	"fmt"
	"os"

	"github.com/ipfs/go-ipfs/filesystem"
	interfaceutils "github.com/ipfs/go-ipfs/filesystem/interfaces"
	gomfs "github.com/ipfs/go-mfs"
)

var _ filesystem.File = (*mfsIOWrapper)(nil)

type mfsIOWrapper struct{ f gomfs.FileDescriptor }

func (mio *mfsIOWrapper) Size() (int64, error)           { return mio.f.Size() }
func (mio *mfsIOWrapper) Read(buff []byte) (int, error)  { return mio.f.Read(buff) }
func (mio *mfsIOWrapper) Write(buff []byte) (int, error) { return mio.f.Write(buff) }
func (mio *mfsIOWrapper) Truncate(size uint64) error     { return mio.f.Truncate(int64(size)) }
func (mio *mfsIOWrapper) Close() error                   { return mio.f.Close() }
func (mio *mfsIOWrapper) Seek(offset int64, whence int) (int64, error) {
	return mio.f.Seek(offset, whence)
}

func (mi *mfsInterface) Open(path string, flags filesystem.IOFlags) (filesystem.File, error) {
	mfsNode, err := gomfs.Lookup(mi.mroot, path)
	if err != nil {
		rErr := &interfaceutils.Error{Cause: err}
		if err == os.ErrNotExist {
			rErr.Type = filesystem.ErrorNotExist
			return nil, rErr
		}
		rErr.Type = filesystem.ErrorPermission
		return nil, rErr
	}

	mfsFileIf, ok := mfsNode.(*gomfs.File)
	if !ok {
		err := fmt.Errorf("%q is not a file (%T)", path, mfsNode)
		return nil, &interfaceutils.Error{Cause: err, Type: filesystem.ErrorIsDir}
	}

	mfsFile, err := mfsFileIf.Open(translateFlags(flags))
	if err != nil {
		return nil, &interfaceutils.Error{Cause: err, Type: filesystem.ErrorPermission}
	}

	return &mfsIOWrapper{f: mfsFile}, nil
}

func translateFlags(flags filesystem.IOFlags) gomfs.Flags {
	switch flags {
	case filesystem.IOReadOnly:
		return gomfs.Flags{Read: true}
	case filesystem.IOWriteOnly:
		return gomfs.Flags{Write: true}
	case filesystem.IOReadWrite:
		return gomfs.Flags{Read: true, Write: true}
	default:
		return gomfs.Flags{}
	}
}
