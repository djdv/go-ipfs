package mfs

import (
	"fmt"

	"github.com/ipfs/go-ipfs/mount/utils/transform"
	"github.com/ipfs/go-mfs"
)

var _ transform.File = (*mfsIOWrapper)(nil)

type mfsIOWrapper struct{ f mfs.FileDescriptor }

func (mio *mfsIOWrapper) Size() (int64, error)           { return mio.f.Size() }
func (mio *mfsIOWrapper) Read(buff []byte) (int, error)  { return mio.f.Read(buff) }
func (mio *mfsIOWrapper) Write(buff []byte) (int, error) { return mio.f.Write(buff) }
func (mio *mfsIOWrapper) Truncate(size uint64) error     { return mio.f.Truncate(int64(size)) }
func (mio *mfsIOWrapper) Close() error                   { return mio.f.Close() }
func (mio *mfsIOWrapper) Seek(offset int64, whence int) (int64, error) {
	return mio.f.Seek(offset, whence)
}

func OpenFile(mroot *mfs.Root, path string, flags transform.IOFlags) (*mfsIOWrapper, error) {
	mfsNode, err := mfs.Lookup(mroot, path)
	if err != nil {
		return nil, err
	}

	mfsFileIf, ok := mfsNode.(*mfs.File)
	if !ok {
		return nil, fmt.Errorf("File IO requested for non-file, type: %v %q", mfsNode.Type(), path)
	}

	mfsFile, err := mfsFileIf.Open(flags.ToMFS())
	if err != nil {
		return nil, err
	}

	return &mfsIOWrapper{f: mfsFile}, nil
}
