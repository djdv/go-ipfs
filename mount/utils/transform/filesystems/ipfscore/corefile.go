package ipfscore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	cbor "github.com/ipfs/go-ipld-cbor"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

var _ transform.File = (*coreFile)(nil)
var _ transform.File = (*cborFile)(nil)

type coreFile struct{ f files.File }

var errRO = errors.New("read only file")

func (cio *coreFile) Size() (int64, error)          { return cio.f.Size() }
func (cio *coreFile) Read(buff []byte) (int, error) { return cio.f.Read(buff) }
func (cio *coreFile) Write(_ []byte) (int, error)   { return 0, errRO }
func (cio *coreFile) Truncate(_ uint64) error       { return errRO }
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
func (cio *cborFile) Write(_ []byte) (int, error)   { return 0, errRO }
func (cio *cborFile) Truncate(_ uint64) error       { return errRO }
func (cio *cborFile) Close() error                  { return nil }
func (cio *cborFile) Seek(offset int64, whence int) (int64, error) {
	return cio.reader.Seek(offset, whence)
}

//func OpenFile(ctx context.Context, ns mountinter.Namespace, path string, core coreiface.CoreAPI, flags transform.IOFlags) (*coreFile, error) {
func OpenFile(ctx context.Context, path corepath.Path, core coreiface.CoreAPI, flags transform.IOFlags) (transform.File, transform.Error) {
	switch flags {
	case transform.IOWriteOnly, transform.IOReadWrite:
		return nil, &transform.ErrorActual{
			GoErr:  errors.New("write request on read-only system"),
			ErrNo:  -fuselib.EROFS,
			P9pErr: errors.New("TODO real error for open-readonly [4a38518d-b861-4654-b821-13867f6d4e71]"),
		}
	}

	ipldNode, err := core.ResolveNode(ctx, path)
	if err != nil {
		return nil, &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.EACCES, // TODO: [review] is this the best value for this?
			P9pErr: errors.New("TODO real error for open-resolve [b6d831b9-982e-4d65-b7c5-38389f34dec5]"),
		}
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
		return nil, &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.EACCES, // TODO: [review] is this the best value for this?
			P9pErr: errors.New("TODO real error for open-unix [63cde99e-a19c-4cc4-84aa-4f64fbbb328c]"),
		}
	}

	fileNode, ok := apiNode.(files.File)
	if !ok {
		// TODO: fill in error string when sensible
		// return nil, fmt.Errorf("%q does not appear to be a file: %T", fullPath.String(), apiNode)
		return nil, &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.EISDIR,
			P9pErr: errors.New("TODO real error for open-file-but-not-a-file [e71a278f-d3ae-493f-b7cf-65cd1fe71c74]"),
		}
	}

	return &coreFile{f: fileNode}, nil
}

func Readlink(ctx context.Context, path corepath.Path, core coreiface.CoreAPI) (string, transform.Error) {
	// make sure the path is actually a link
	iStat, _, err := transform.GetAttr(ctx, path, core, transform.IPFSStatRequest{Type: true})
	if err != nil {
		return "", &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.ENOENT,
			P9pErr: err,
		}
	}

	if iStat.FileType != coreiface.TSymlink {
		return "", &transform.ErrorActual{
			GoErr:  fmt.Errorf("%q is not a symlink", path),
			ErrNo:  -fuselib.EINVAL,
			P9pErr: err,
		}
	}

	// if it is, read it
	linkNode, err := core.Unixfs().Get(ctx, path)
	if err != nil {
		return "", &transform.ErrorActual{
			GoErr:  err,
			ErrNo:  -fuselib.EIO,
			P9pErr: err,
		}
	}

	// NOTE: the implementation of this does no type checks
	// which is why we check the node's type above
	linkActual := files.ToSymlink(linkNode)

	return linkActual.Target, nil
}
