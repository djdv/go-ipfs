package meta

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/hugelgupf/p9/p9"
)

// WalkRef is used to generalize 9P `Walk` operations within file system boundaries
// and allows for traversal across those boundaries if intended by the implementation
// Only a subset of the 9P file protocol semantics will be noted here
// Please see `walk(5)` for more information on the standard
type WalkRef interface {
	p9.File

	/* IsOpen returns true if the reference node has been opened for I/O (by any reference)*/
	//TODO: IsOpen() bool

	/* IsClosed returns true if the reference itself has been closed */
	IsClosed() bool

	/* Fork allocates a new reference `newfid`, derived from the existing reference `fid`

	The returned reference node must exist parallel to the existing `WalkRef`/`fid`
	(e.g. In a path based index, a forked node would contain the same path as its origin node)

	The returned node must also adhere to 'walk(5)' `newfid` semantics
	Meaning that...
	`newfid` must be allowed to `Close` separately from the original reference
	`newfid`'s path may be modified during `Walk` without affecting the original `WalkRef`
	`Open` must flag all references within the same system, at the same path, as open
	etc. in compliance with 'walk(5)'
	*/
	Fork() (WalkRef, error)

	/* QID returns the QID for the node's current path, if a file exists there and can be accessed */
	QID() (p9.QID, error)

	/* Step should return a reference that is tracking the result of
	the node's current-path + "name"
	Step should make sure that the current reference adheres to the restrictions
	of 'walk(5)'
	In particular the reference must not be open for I/O, or otherwise already closed
	etc.
	*/
	Step(name string) (WalkRef, error)

	/* Backtrack is the handler for `..` requests
	it is effectively the implicit inverse of `Step`
	if called on the root node, the node should return itself
	*/
	Backtrack() (parentRef WalkRef, err error)
}

type OverlayBase struct {

	/* The parent context must be set prior to `fs.Attach`. */
	ParentCtx context.Context

	/*	When a new reference to `fs` is created during `fs.Attach`,
		it must populate the fs-context+cancel pair with new values derived
		from the pre-existing parent context.

		This context must be canceled when the reference's `Close` method is called
		i.e. it should be valid only for the lifetime of the file system
	*/
	FilesystemCtx    context.Context
	FilesystemCancel context.CancelFunc

	/*	When a new reference to `fs` is created during `fs.Walk`,
		it must populate the op-context+cancel pair with new values derived
		from the pre-existing `FilesystemCtx`.

		This context must be canceled when the reference's `Close` method is called
		i.e. it should be valid only for the lifetime of the file reference
	*/
	OperationsCtx    context.Context
	OperationsCancel context.CancelFunc

	/* Must be set to true upon `Close`, invalidates all future operations for this reference */
	Closed bool

	/* Atomic value;
	Must be set to non-zero if any reference opens I/O
	and reset to 0 upon `Close` of the open reference */
	Opened *uintptr
}

// Clone returns a copy of OverlayBase with cancel functions omitted
func (ob *OverlayBase) Clone() OverlayBase {
	return OverlayBase{
		ParentCtx:     ob.ParentCtx,
		FilesystemCtx: ob.FilesystemCtx,
		Closed:        ob.Closed,
		Opened:        ob.Opened,
	}
}

func (ob *OverlayBase) CallCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(ob.FilesystemCtx, 30*time.Second)
}

func (ob *OverlayBase) IsOpen() bool {
	return atomic.LoadUintptr(ob.Opened) > 0
}

func (ob *OverlayBase) IsClosed() bool {
	return ob.Closed
}
