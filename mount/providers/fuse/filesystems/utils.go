package fusecommon

import (
	"fmt"
	gopath "path"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

type fillFunc func(name string, stat *fuselib.Stat_t, ofst int64) bool

func FillDir(directory transform.Directory, writable bool, fill fillFunc, offset int64) (error, int) {
	// TODO: [audit] int -> uint needs range checking
	entChan, err := directory.Readdir(uint64(offset), 0).ToFuse()
	if err != nil {
		// TODO: inspect/transform error
		return err, -fuselib.ENOENT
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
		// stat will always be nil on platforms that have ReaddirPlus disabled
		// and is not gauranteed to be filled on those that do
		if ent.Stat != nil {
			ApplyPermissions(writable, &ent.Stat.Mode)
		}

		if !fill(ent.Name, ent.Stat, fillOffset+ent.Offset) {
			break
		}
	}
	return nil, OperationSuccess
}

func JoinRoot(ns mountinter.Namespace, path string) (corepath.Path, error) {
	var rootPath string
	switch ns {
	default:
		return nil, fmt.Errorf("unsupported namespace: %s", ns.String())
	case mountinter.NamespaceIPFS:
		rootPath = "/ipfs"
	case mountinter.NamespaceIPNS:
		rootPath = "/ipns"
	case mountinter.NamespaceCore:
		rootPath = "/ipld"
	}
	return corepath.New(gopath.Join(rootPath, path)), nil
}

// TODO: this is mostly here as a placeholder/marker until we figure out how best to standardize permissions
// not everything should have the execute bit set but this isn't stored anywhere for us to fetch either
func ApplyPermissions(fsWritable bool, mode *uint32) {
	*mode |= IRXA &^ (fuselib.S_IXOTH) // |0554
	if fsWritable {
		*mode |= (fuselib.S_IWGRP | fuselib.S_IWUSR) // |0220
	}
}
