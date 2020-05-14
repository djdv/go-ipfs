package ipfscore

import (
	"bytes"
	"context"
	"io"

	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	cbor "github.com/ipfs/go-ipld-cbor"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

var _ transform.File = (*coreFile)(nil)
var _ transform.File = (*cborFile)(nil)

type coreFile struct{ f files.File }

func (cio *coreFile) Size() (int64, error)          { return cio.f.Size() }
func (cio *coreFile) Read(buff []byte) (int, error) { return cio.f.Read(buff) }
func (cio *coreFile) Write(_ []byte) (int, error)   { return 0, transform.ErrIOReadOnly }
func (cio *coreFile) Truncate(_ uint64) error       { return transform.ErrIOReadOnly }
func (cio *coreFile) Close() error                  { return cio.f.Close() }
func (cio *coreFile) Seek(offset int64, whence int) (int64, error) {
	return cio.f.Seek(offset, whence)
}

type cborFile struct {
	node   *cbor.Node
	reader io.ReadSeeker
}

func (cio *cborFile) Size() (int64, error) {
	size, err := cio.node.Size()
	return int64(size), err
}
func (cio *cborFile) Read(buff []byte) (int, error) { return cio.reader.Read(buff) }
func (cio *cborFile) Write(_ []byte) (int, error)   { return 0, transform.ErrIOReadOnly }
func (cio *cborFile) Truncate(_ uint64) error       { return transform.ErrIOReadOnly }
func (cio *cborFile) Close() error                  { return nil }
func (cio *cborFile) Seek(offset int64, whence int) (int64, error) {
	return cio.reader.Seek(offset, whence)
}

//func OpenFile(ctx context.Context, ns mountinter.Namespace, path string, core coreiface.CoreAPI, flags transform.IOFlags) (*coreFile, error) {
func OpenFile(ctx context.Context, path corepath.Path, core coreiface.CoreAPI, flags transform.IOFlags) (transform.File, error) {
	switch flags {
	case transform.IOWriteOnly, transform.IOReadWrite:
		return nil, &transform.ErrIOReadOnly
	}

	ipldNode, err := core.ResolveNode(ctx, path)
	if err != nil {
		return nil, err
	}

	// NOTE: for now we just return the actual cbor
	// we'll wait for someone else to implement file and directory interfaces on them in UnixFS
	if cborNode, ok := ipldNode.(*cbor.Node); ok {
		br := bytes.NewReader(cborNode.RawData())
		/* 	TODO [review]
		we could also retrun this as human readable JSON
		but I'm not sure which is prefferable
			forHumans, err := cborNode.MarshalJSON()
			if err != nil {
				return nil, err
			}
			br := bytes.NewReader(forHumans)
		*/
		return &cborFile{node: cborNode, reader: br}, nil
	}

	apiNode, err := core.Unixfs().Get(ctx, path)
	if err != nil {
		return nil, &transform.IOError{ExternalErr: err}
	}

	fileNode, ok := apiNode.(files.File)
	if !ok {
		// TODO: fill in error string when sensible
		// return nil, fmt.Errorf("%q does not appear to be a file: %T", fullPath.String(), apiNode)
		return nil, &transform.ErrNotFile
	}

	return &coreFile{f: fileNode}, nil
}
