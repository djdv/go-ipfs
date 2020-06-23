package ipfscore

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-ipfs/filesystem"
	interfaceutils "github.com/ipfs/go-ipfs/filesystem/interfaces"
	cbor "github.com/ipfs/go-ipld-cbor"
)

var _, _ filesystem.File = (*coreFile)(nil), (*cborFile)(nil)

type coreFile struct{ f files.File }

func (cio *coreFile) Size() (int64, error)          { return cio.f.Size() }
func (cio *coreFile) Read(buff []byte) (int, error) { return cio.f.Read(buff) }
func (cio *coreFile) Write(_ []byte) (int, error)   { return 0, errNotImplemented }
func (cio *coreFile) Truncate(_ uint64) error       { return errNotImplemented }
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
func (cio *cborFile) Write(_ []byte) (int, error)   { return 0, errNotImplemented }
func (cio *cborFile) Truncate(_ uint64) error       { return errNotImplemented }
func (cio *cborFile) Close() error                  { return nil }
func (cio *cborFile) Seek(offset int64, whence int) (int64, error) {
	return cio.reader.Seek(offset, whence)
}

func (ci *coreInterface) Open(path string, flags filesystem.IOFlags) (filesystem.File, error) {
	if flags != filesystem.IOReadOnly {
		return nil, &interfaceutils.Error{
			Cause: errors.New("read only FS"),
			Type:  filesystem.ErrorReadOnly,
		}
	}

	corePath := ci.joinRoot(path)

	callCtx, callCancel := interfaceutils.CallContext(ci.ctx)
	defer callCancel()
	ipldNode, err := ci.core.ResolveNode(callCtx, corePath)
	if err != nil {
		return nil, &interfaceutils.Error{
			Cause: err,
			Type:  filesystem.ErrorPermission,
		}
	}

	// special handling for cbor nodes
	if cborNode, ok := ipldNode.(*cbor.Node); ok {
		br := bytes.NewReader(cborNode.RawData())
		return &cborFile{node: cborNode, reader: br}, nil
		// TODO [review] we could return this as human readable JSON instead of the raw data
		// but I'm not sure which is prefferable
		/*
			forHumans, err := cborNode.MarshalJSON()
			if err != nil {
				return nil, err
			}
			br := bytes.NewReader(forHumans)
		*/
	}

	apiNode, err := ci.core.Unixfs().Get(ci.ctx, corePath)
	if err != nil {
		return nil, &interfaceutils.Error{
			Cause: err,
			Type:  filesystem.ErrorPermission,
		}
	}

	fileNode, ok := apiNode.(files.File)
	if !ok {
		err := fmt.Errorf("%q does not appear to be a file: %T", path, apiNode)
		return nil, &interfaceutils.Error{
			Cause: err,
			Type:  filesystem.ErrorIsDir,
		}
	}

	return &coreFile{f: fileNode}, nil
}