package keyfs

import (
	"fmt"
	"io"

	transform "github.com/ipfs/go-ipfs/filesystem"
	transcom "github.com/ipfs/go-ipfs/filesystem/interfaces"
	"github.com/ipfs/go-ipfs/filesystem/interfaces/mfs"
	"github.com/ipfs/go-merkledag"
	gomfs "github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

// rootRef wraps a foreign file system
// with means to manage sub-references of that system
type rootRef struct {
	transform.Interface
	counter refCounter
	//io.Closer
}

// root references must be closed when no longer used
// otherwise they'll remain in the active table
func (rr rootRef) Close() error { return rr.counter.decrement() }

// sub references must also be closed when no longer used
// for the same reason
type (
	rootFileRef struct {
		transform.File
		io.Closer
	}
	rootDirectoryRef struct {
		transform.Directory
		io.Closer
	}
)

func (rf rootFileRef) Close() error      { return rf.Closer.Close() }
func (rd rootDirectoryRef) Close() error { return rd.Closer.Close() }

func rootCloserGen(rootRef *rootRef, subRef io.Closer) closer {
	return func() error {
		err := subRef.Close()               // `Close` the subreference itself
		rErr := rootRef.counter.decrement() // remove association with its superior
		if err == nil && rErr != nil {      // returning the supererror, only if there is no suberror
			err = rErr
		}
		return err
	}
}

// `Open` overrides the native system's `Open` method
// adding in reference tracking to a shared instance of the system
func (rr rootRef) Open(path string, flags transform.IOFlags) (transform.File, error) {
	rr.counter.increment()
	file, err := rr.Interface.Open(path, flags)
	if err != nil {
		rr.counter.decrement() // we know we're not the last reference so the error is unchecked
		return nil, err
	}

	return rootFileRef{
		File:   file,
		Closer: rootCloserGen(&rr, file),
	}, nil
}

func (rr rootRef) OpenDirectory(path string) (transform.Directory, error) {
	rr.counter.increment()
	directory, err := rr.Interface.OpenDirectory(path)
	if err != nil {
		rr.counter.decrement() // we know we're not the last reference so the error is unchecked
		return nil, err
	}

	return &rootDirectoryRef{
		Directory: directory,
		Closer:    rootCloserGen(&rr, directory),
	}, nil
}

func (ki *keyInterface) getRoot(key coreiface.Key) (transform.Interface, error) {
	return ki.references.getRootRef(key.Name(), func() (transform.Interface, error) {
		mroot, err := ki.keyToMFSRoot(key)
		if err != nil {
			return nil, err
		}

		return mfs.NewInterface(ki.ctx, mroot), nil
	})
}

func (ki *keyInterface) keyToMFSRoot(key coreiface.Key) (*gomfs.Root, error) {
	callCtx, cancel := transcom.CallContext(ki.ctx)
	defer cancel()

	path, err := ki.core.ResolvePath(callCtx, key.Path())
	if err != nil {
		return nil, err
	}

	ipldNode, err := ki.core.ResolveNode(callCtx, path)
	if err != nil {
		return nil, err
	}

	iStat, _, err := ki.core.Stat(callCtx, path, transform.IPFSStatRequest{Type: true})
	if err != nil {
		return nil, err
	}

	if iStat.FileType != coreiface.TDirectory {
		err := fmt.Errorf("key %q is not a directory (type: %s)", key.Name(), iStat.FileType.String())
		return nil, &transcom.Error{Cause: err, Type: transform.ErrorNotDir}
	}

	pbNode, ok := ipldNode.(*merkledag.ProtoNode)
	if !ok {
		err := fmt.Errorf("key %q has incompatible root node type (%T)", key.Name(), ipldNode)
		return nil, &transcom.Error{Cause: err, Type: transform.ErrorInvalidItem}
	}

	mroot, err := gomfs.NewRoot(ki.ctx, ki.core.Dag(), pbNode, ki.publisherGenMFS(key.Name()))
	if err != nil {
		return nil, &transcom.Error{Cause: err, Type: transform.ErrorIO}
	}
	return mroot, nil
}
