package mountcom

import (
	"crypto/rand"
	"errors"
	"hash/fnv"
	"io"
	"strings"
	"time"

	mountinter "github.com/ipfs/go-ipfs/mount/interface"
)

// TODO: everything in here is a quick hack just to get something implemented
// there's probably more efficient and sane ways to implement ancestry or other fs-path based locks

const saltSize = 32

var hashGeneratorSalt []byte

func init() {
	hashGeneratorSalt = make([]byte, saltSize)
	_, err := io.ReadFull(rand.Reader, hashGeneratorSalt)
	if err != nil {
		panic(err)
	}
}

type LockType uint

//go:generate stringer -type=LockType -trimprefix=Lock
const (
	lockInternal LockType = iota
	LockDataRead
	LockDataWrite
	LockMetaRead
	LockMetaWrite
	LockDirectoryRead
	LockDirectoryWrite
)

// TODO: docs
type ResourceLock interface {
	Request(namespace mountinter.Namespace, resourceReference string, ltype LockType, timeout time.Duration) error
	Release(namespace mountinter.Namespace, resourceReference string, ltype LockType)
}

// looseLock uses runtime channel syncing semantics to create a 2 phase "try-lock"
type looseLock struct {
	l chan struct{} // embed in a struct because apparently typed chans can't be sent to and aliaes can't have methods bound
}

func newTryLock() *looseLock {
	// create a buffered channel for the locktype
	// allowing 1 value to proceed before blocking on next; used for try-lock semantic
	// pad the buffer to acquire the lock:
	// l <- struct{}{}
	// flush buffer to release it:
	// <-l
	return &looseLock{l: make(chan struct{}, 1)}
}

func (l *looseLock) tryLock(timeout time.Duration) error {
	select { // try-lock
	case l.l <- struct{}{}:
		// we were able to fill the buffer; lock acquired by caller
		// subsequent calls will block until unlocked
		return nil
	case <-time.After(timeout):
		return errors.New("timed out")
	}
}

func (l *looseLock) unlock() {
	if len(l.l) > 0 {
		// if the lock was previously acquired, the buffer should be filled
		// dump it to release the lock
		<-l.l
	} else {
		panic("unlock called when not locked")
	}
}

type trylock interface {
	tryLock(time.Duration) error
	unlock()
}

// XXX: kind of insane; might not need to be this complex
type lockNode struct {
	// parent   *lockNode // we don't use this yet
	children map[string]*lockNode // TODO: [consider] {label string; children[]*lockNode} vs builtin map (should perform nicer and be less annoying to alloc)
	locks    map[LockType]trylock // TODO: explode this out from a map to static members when lock types stabilize
}

func newLockNode() *lockNode {
	return &lockNode{
		children: make(map[string]*lockNode),
		locks: map[LockType]trylock{ // TODO: this is pretty gross; reconsider approach
			lockInternal:       newTryLock(),
			LockDataRead:       newTryLock(),
			LockDataWrite:      newTryLock(),
			LockMetaRead:       newTryLock(),
			LockMetaWrite:      newTryLock(),
			LockDirectoryRead:  newTryLock(),
			LockDirectoryWrite: newTryLock(),
		},
	}
}

type resLock struct {
	//nodeMu            sync.Mutex // [silly] lock for our locks; we don't want concurrent node manipulation
	ipfs, ipns, files *lockNode
	//referenceBag map[mountinter.Namespace]map[uint64]map[LockType]looseLock
}

func NewResourceLocker() ResourceLock {
	return &resLock{
		ipfs:  newLockNode(),
		ipns:  newLockNode(),
		files: newLockNode(),
	}
}

func (rl *resLock) Request(namespace mountinter.Namespace, resourceReference string, ltype LockType, timeout time.Duration) error {
	//rl.Lock()
	//defer rl.Unlock()

	// grab the root lock actual for the namespace
	var lRoot *lockNode
	switch namespace {
	case mountinter.NamespaceIPFS:
		lRoot = rl.ipfs
	case mountinter.NamespaceIPNS:
		lRoot = rl.ipns
	case mountinter.NamespaceFiles:
		lRoot = rl.files
	}

	// drill down to the final node
	l, unwind, err := drill(lRoot, timeout, resourceReference)
	if err != nil {
		return err
	}
	defer unwind()

	// lock final node
	// TODO: this is going to require some complex nonsense of locking the parent(s) depending on the lock type
	// we're likely going to have to create an unlock closure and stick it on one of the lock objects or return it to the caller

	return l.locks[ltype].tryLock(timeout)
}

func drill(lRoot *lockNode, timeout time.Duration, resourceReference string) (*lockNode, func(), error) {
	// obtain or initialize the lock groups
	if err := lRoot.locks[lockInternal].tryLock(timeout); err != nil {
		return nil, nil, err
	}

	unwindStack := []func(){lRoot.locks[lockInternal].unlock}
	unwind := func() {
		for _, unlock := range unwindStack {
			unlock()
		}
	}

	l := lRoot
	for _, comp := range strings.Split(resourceReference, "/") {
		child, ok := l.children[comp]
		if !ok {
			child = newLockNode()
			l.children[comp] = child
		}
		l = child
		if err := l.locks[lockInternal].tryLock(timeout); err != nil {
			unwind()
			return nil, nil, err
		}
		unwindStack = append(unwindStack, l.locks[lockInternal].unlock)
	}
	return l, unwind, nil
}

// TODO: [audit]; hastily written; deadlock, leak, and panic potential
func (rl *resLock) Release(namespace mountinter.Namespace, resourceReference string, ltype LockType) {
	//rl.Lock()
	//defer rl.Unlock()

	// grab the root lock actual for the namespace
	var lRoot *lockNode
	switch namespace {
	case mountinter.NamespaceIPFS:
		lRoot = rl.ipfs
	case mountinter.NamespaceIPNS:
		lRoot = rl.ipns
	case mountinter.NamespaceFiles:
		lRoot = rl.files
	}

	// drill down to the final node
	l, unwind, err := drill(lRoot, 100, resourceReference)
	if err != nil {
		panic(err)
	}
	defer unwind()

	// unlock final node
	// TODO: this is going to require some complex nonsense of locking the parent(s) depending on the lock type
	// we're likely going to have to create an unlock closure and stick it on one of the lock objects or return it to the caller

	l.locks[ltype].unlock()
}

func hashRes(in string) uint64 {
	hasher := fnv.New64a()

	if _, err := hasher.Write(hashGeneratorSalt); err != nil {
		panic(err)
	}

	if _, err := hasher.Write([]byte(in)); err != nil {
		panic(err)
	}
	return hasher.Sum64()
}
