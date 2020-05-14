package mfs

import (
	"errors"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
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

func OpenFile(mroot *mfs.Root, path string, flags transform.IOFlags) (*mfsIOWrapper, transform.Error) {
	mfsNode, err := mfs.Lookup(mroot, path)
	if err != nil {
		return nil, &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.EACCES, // TODO: [review] is this the best value for this?
			P9pErr: errors.New("TODO real error for open-lookup [9768fef2-4795-4aba-9009-497c55f3cb72]"),
		}
	}

	mfsFileIf, ok := mfsNode.(*mfs.File)
	if !ok {
		return nil, &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.EISDIR,
			P9pErr: errors.New("TODO real error for open-file-but-not-a-file [cef4642f-2863-4e4a-8e52-9d5a5a687b10]"),
		}
	}

	mfsFile, err := mfsFileIf.Open(flags.ToMFS())
	if err != nil {
		return nil, &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.EACCES, // TODO: [review] is this the best value for this?
			P9pErr: errors.New("TODO real error for open-failure [16a35016-405a-414b-82d2-579eb8712398]"),
		}
	}

	return &mfsIOWrapper{f: mfsFile}, nil
}
