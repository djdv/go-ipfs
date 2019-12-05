package fsutils

import (
	"github.com/hugelgupf/p9/p9"
	fserrors "github.com/ipfs/go-ipfs/plugin/plugins/filesystem/errors"
	"github.com/ipfs/go-ipfs/plugin/plugins/filesystem/meta"
)

// Walker implements the 9P `Walk` operation
func Walker(ref meta.WalkRef, names []string) ([]p9.QID, p9.File, error) {
	if ref.IsClosed() {
		return nil, nil, fserrors.FileClosed
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
