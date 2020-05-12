package keyfs

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/ipfs/go-ipfs/mount/utils/transform"
	coreiface "github.com/ipfs/interface-go-ipfs-core"

	chunk "github.com/ipfs/go-ipfs-chunker"
	"github.com/ipfs/go-unixfs/mod"
)

var _ transform.File = (*keyFile)(nil)

// TODO: [review] there's a lot of indirection going on here, some of which might not be necessary
// if we can get away with less, do so, but we also might not be able to avoid it

// TODO: [review] comments are written in AM PD English; make another pass

func NewFileWrapper(ctx context.Context, core coreiface.CoreAPI) *FileWrapper {
	return &FileWrapper{
		ctx:      ctx,
		core:     core,
		dagTable: make(dagTable)}
}

type (
	dagTable map[string]*dagRef

	// TODO: since this is exported it'd be better as an interface
	// responsible for assigning an underlying dag modifier to a key by its name
	FileWrapper struct {
		sync.Mutex // gaurd table access
		dagTable   dagTable

		ctx  context.Context // should be valid for as long as files intend to be accessed via this table
		core coreiface.CoreAPI
	}
)

type (
	// multiple file descriptors to the same key will share the same underlying dag modifer
	// so that they may stay in sync with eachother
	dagRef struct {
		sync.Mutex // gaurd access to the modifier's methods
		*mod.DagModifier
		refCount uint
	}

	// the underlyng dag modifier is a single descriptor with its own cursor
	// we want to share its buffer, but need our own unique cursor for each of our own descriptors
	keyFile struct {
		dag    *dagRef
		cursor int64
		closer func() error
	}
)

func (kio *keyFile) Size() (int64, error) {
	// NOTE: this could be a read lock since Size shouldn't modify the dagmod
	// but a rwmutex doesn't seem worth it for single short op
	kio.dag.Lock()
	defer kio.dag.Unlock()
	return kio.dag.Size()
}
func (kio *keyFile) Read(buff []byte) (int, error) {
	kio.dag.Lock()
	defer kio.dag.Unlock()
	return kio.dag.Read(buff)
}
func (kio *keyFile) Write(buff []byte) (int, error) {
	kio.dag.Lock()
	defer kio.dag.Unlock()
	return kio.dag.Write(buff)
}
func (kio *keyFile) Close() error { return kio.closer() }
func (kio *keyFile) Seek(offset int64, whence int) (int64, error) {
	// NOTE: same note as in Size()
	kio.dag.Lock()
	defer kio.dag.Unlock()

	switch whence {
	case io.SeekStart:
		if offset < 0 {
			return kio.cursor, errors.New("tried to seek to a position before the beginning of the file")
		}
		kio.cursor = offset
	case io.SeekCurrent:
		kio.cursor += offset
	case io.SeekEnd:
		end, err := kio.dag.Size()
		if err != nil {
			return kio.cursor, err
		}
		kio.cursor = end + offset
	}

	return kio.cursor, nil
}

// TODO: parse flags and limit functionality contextually (RO, WO, etc.)
// for now we always give full access
func (ft FileWrapper) Open(key coreiface.Key, _ transform.IOFlags) (transform.File, error) {
	ft.Lock()
	defer ft.Unlock()

	keyName := key.Name()

	if dagRef, ok := ft.dagTable[keyName]; ok {
		dagRef.refCount++
		closer := func() error {
			ft.Lock()
			defer ft.Unlock()
			dagRef.refCount--
			if dagRef.refCount == 0 {
				delete(ft.dagTable, keyName)
			}
			return nil
		}

		return &keyFile{dag: dagRef, closer: closer}, nil
	}

	ipldNode, err := ft.core.ResolveNode(ft.ctx, key.Path())
	if err != nil {
		return nil, err
	}

	dmod, err := mod.NewDagModifier(ft.ctx, ipldNode, ft.core.Dag(), func(r io.Reader) chunk.Splitter {
		return chunk.NewBuzhash(r)
	})
	if err != nil {
		return nil, err
	}

	dagRef := &dagRef{DagModifier: dmod, refCount: 1}
	ft.dagTable[keyName] = dagRef

	closer := func() error {
		ft.Lock()
		defer ft.Unlock()
		dagRef.refCount--
		if dagRef.refCount == 0 {
			delete(ft.dagTable, key.Name())
		}
		return nil
	}

	return &keyFile{dag: dagRef, closer: closer}, nil
}
