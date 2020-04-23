package fusecommon

import (
	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
)

type fillFunc func(name string, stat *fuselib.Stat_t, ofst int64) bool

func FillDir(directory transform.Directory, writable bool, fill fillFunc, offset int64) (error, int) {
	// TODO: [audit] int -> uint needs range checking
	entChan, err := directory.Readdir(uint64(offset), 0).ToFuse()
	if err != nil {
		// TODO: inspect/transform error
		return err, -fuselib.EBADF
	}

	// dots are optional in POSIX but everyone expects them
	// lots of things break without them so we use them
	// NOTE: we let the OS populate the stats because it's not worth the complexity yet
	// later this may change to add 2 closed procedures for self/parent; self|parentResolve()(*stat, error)
	switch offset {
	case 0:
		if !fill(".", nil, 1) {
			return nil, OperationSuccess
		}
		fallthrough
	case 1:
		if !fill("..", nil, 2) {
			return nil, OperationSuccess
		}
	}

	// offset 0 has special meaning in FUSE
	// so all offset values in our API are expected to be non-0
	// more specifically, they're expected to start at 1 and increase incrementally
	// we account for our dots as taking offset positions 1 and 2 in every directory
	// we'll then sum our local offset with the offset of the independent entries
	// to result in the final offset returned to FUSE
	var fillOffset int64 = 2

	for ent := range entChan {
		ApplyPermissions(writable, &ent.Stat.Mode)
		if !fill(ent.Name, ent.Stat, fillOffset+ent.Offset) {
			break
		}
	}
	return nil, OperationSuccess
}
