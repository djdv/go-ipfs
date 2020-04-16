package transform

import (
	"context"
	"fmt"
	"io"

	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

/* TODO: something like
type File = (go-ipfs-files).File
func MFSOpenFile(...) File
*/

type File interface {
	io.ReadWriteCloser
	io.Seeker
	Size() (int64, error)
}

type coreIOWrapper struct{ f files.File }

func (cio *coreIOWrapper) Size() (int64, error)           { return cio.f.Size() }
func (cio *coreIOWrapper) Read(buff []byte) (int, error)  { return cio.f.Read(buff) }
func (cio *coreIOWrapper) Write(buff []byte) (int, error) { return 0, ErrIOReadOnly }
func (cio *coreIOWrapper) Close() error                   { return cio.f.Close() }
func (cio *coreIOWrapper) Seek(offset int64, whence int) (int64, error) {
	return cio.f.Seek(offset, whence)
}

type mfsIOWrapper struct{ f mfs.FileDescriptor }

func (mio *mfsIOWrapper) Size() (int64, error)           { return mio.f.Size() }
func (mio *mfsIOWrapper) Read(buff []byte) (int, error)  { return mio.f.Read(buff) }
func (mio *mfsIOWrapper) Write(buff []byte) (int, error) { return mio.f.Write(buff) }
func (mio *mfsIOWrapper) Close() error                   { return mio.f.Close() }
func (mio *mfsIOWrapper) Seek(offset int64, whence int) (int64, error) {
	return mio.f.Seek(offset, whence)
}

func CoreOpenFile(ctx context.Context, path corepath.Path, core coreiface.CoreAPI, flags IOFlags) (File, error) {
	switch flags {
	case IOWriteOnly, IOReadWrite:
		return nil, ErrIOReadOnly
	}

	apiNode, err := core.Unixfs().Get(ctx, path)
	if err != nil {
		return nil, err
	}

	fileNode, ok := apiNode.(files.File)
	if !ok {
		return nil, fmt.Errorf("%q does not appear to be a file: %T", path.String(), apiNode)
	}

	return &coreIOWrapper{f: fileNode}, nil
}

func MFSOpenFile(mroot *mfs.Root, path string, flags IOFlags) (File, error) {
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
