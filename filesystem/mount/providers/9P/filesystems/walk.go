package common

import (
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

// Walker implements the 9P `Walk` operation
func Walker(ref WalkRef, names []string) ([]p9.QID, p9.File, error) {
	if ref.IsClosed() {
		return nil, nil, FileClosed
	}
	// no matter the outcome, we start with a `newfid`
	curRef, err := ref.Fork()
	if err != nil {
		return nil, nil, err
	}

	if shouldClone(names) {
		qid, err := ref.QID() // validate the node is "walkable"
		if err != nil {
			return nil, nil, err
		}
		return []p9.QID{qid}, curRef, nil
	}

	qids := make([]p9.QID, 0, len(names))

	for _, name := range names {
		switch name {
		default:
			// get ready to step forward; maybe across FS bounds
			curRef, err = curRef.Step(name)

		case ".":
			// don't prepare to move at all

		case "..":
			// get ready to step backwards; maybe across FS bounds
			curRef, err = curRef.Backtrack()
		}

		if err != nil {
			return qids, nil, err
		}

		// commit to the step
		qid, err := curRef.QID()
		if err != nil {
			return qids, nil, err
		}

		// set on success, we stepped forward
		qids = append(qids, qid)
	}

	return qids, curRef, nil
}

/* walk(5):
It is legal for `nwname` to be zero, in which case `newfid` will represent the same `file` as `fid`
and the `walk` will usually succeed; this is equivalent to walking to dot.
*/
func shouldClone(names []string) bool {
	switch len(names) {
	case 0: // truly empty path
		return true
	case 1: // self or empty but not nil
		pc := names[0]
		return pc == "." || pc == ""
	default:
		return false
	}
}
