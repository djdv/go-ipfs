package mfs

import (
	"fmt"
	"os"

	"github.com/ipfs/go-ipfs/mount/utils/transform"
	transcom "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems"
	gomfs "github.com/ipfs/go-mfs"
)

var _ transform.File = (*mfsIOWrapper)(nil)

type mfsIOWrapper struct{ f gomfs.FileDescriptor }

func (mio *mfsIOWrapper) Size() (int64, error)           { return mio.f.Size() }
func (mio *mfsIOWrapper) Read(buff []byte) (int, error)  { return mio.f.Read(buff) }
func (mio *mfsIOWrapper) Write(buff []byte) (int, error) { return mio.f.Write(buff) }
func (mio *mfsIOWrapper) Truncate(size uint64) error     { return mio.f.Truncate(int64(size)) }
func (mio *mfsIOWrapper) Close() error                   { return mio.f.Close() }
func (mio *mfsIOWrapper) Seek(offset int64, whence int) (int64, error) {
	return mio.f.Seek(offset, whence)
}

func (mi *mfsInterface) Open(path string, flags transform.IOFlags) (transform.File, error) {
	mfsNode, err := gomfs.Lookup(mi.mroot, path)
	if err != nil {
		rErr := &transcom.Error{Cause: err}
		if err == os.ErrNotExist {
			rErr.Type = transform.ErrorNotExist
			return nil, rErr
		}
		rErr.Type = transform.ErrorPermission
		return nil, rErr
	}

	mfsFileIf, ok := mfsNode.(*gomfs.File)
	if !ok {
		err := fmt.Errorf("%q is not a file (%T)", path, mfsNode)
		return nil, &transcom.Error{Cause: err, Type: transform.ErrorIsDir}
	}

	mfsFile, err := mfsFileIf.Open(translateFlags(flags))
	if err != nil {
		return nil, &transcom.Error{Cause: err, Type: transform.ErrorPermission}
	}

	return &mfsIOWrapper{f: mfsFile}, nil
}

func translateFlags(flags transform.IOFlags) gomfs.Flags {
	switch flags {
	case transform.IOReadOnly:
		return gomfs.Flags{Read: true}
	case transform.IOWriteOnly:
		return gomfs.Flags{Write: true}
	case transform.IOReadWrite:
		return gomfs.Flags{Read: true, Write: true}
	default:
		return gomfs.Flags{}
	}
}
