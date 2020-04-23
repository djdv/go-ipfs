package ipfscore

import (
	"context"
	"fmt"

	files "github.com/ipfs/go-ipfs-files"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var _ transform.File = (*coreFile)(nil)

type coreFile struct{ f files.File }

func (cio *coreFile) Size() (int64, error)           { return cio.f.Size() }
func (cio *coreFile) Read(buff []byte) (int, error)  { return cio.f.Read(buff) }
func (cio *coreFile) Write(buff []byte) (int, error) { return 0, transform.ErrIOReadOnly }
func (cio *coreFile) Close() error                   { return cio.f.Close() }
func (cio *coreFile) Seek(offset int64, whence int) (int64, error) {
	return cio.f.Seek(offset, whence)
}

func OpenFile(ctx context.Context, ns mountinter.Namespace, path string, core coreiface.CoreAPI, flags transform.IOFlags) (*coreFile, error) {
	fullPath, err := joinRoot(ns, path)
	if err != nil {
		return nil, err
	}

	switch flags {
	case transform.IOWriteOnly, transform.IOReadWrite:
		return nil, transform.ErrIOReadOnly
	}

	apiNode, err := core.Unixfs().Get(ctx, fullPath)
	if err != nil {
		return nil, err
	}

	fileNode, ok := apiNode.(files.File)
	if !ok {
		return nil, fmt.Errorf("%q does not appear to be a file: %T", fullPath.String(), apiNode)
	}

	return &coreFile{f: fileNode}, nil
}
