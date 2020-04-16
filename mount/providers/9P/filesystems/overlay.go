package common

import (
	"context"
	"sync/atomic"
	"time"
)

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
