package fusecommon

import (
	"errors"
	"fmt"
	gopath "path"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

type fillFunc func(name string, stat *fuselib.Stat_t, ofst int64) bool

func FillDir(directory transform.Directory, writable bool, fill fillFunc, offset int64) (error, int) {
	// dots are optional in POSIX but lots of things break without them, so we fill them in
	// NOTE: we let the OS populate the stats because it's not worth the complexity yet
	// later this may change to add 2 closed procedures for self/parent; `{self|parent}Resolve()(*stat, error)`

	// Returning entries with offset value 0 has a special meaning in FUSE
	// so all returned offsets values are expected to be 0>
	// FillDir expects the input directory to follow this convention, and supply us with offsets 0>
	// to avoid overlap, or range requirements
	// we sum our local (dot) offset with the entry's offset to get a value suitable to return
	// and do the inverse to get the directory's input offset value (from a value we previously returned)
	// rel: SUSv7 `readdir`, `seekdir`, `telldir`

	const dotOffsetBase = 2 // 0th index for Readdir return values

	var relativeOffset uint64 // offset used for input, adjusting for dots if any

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
	default:
		// TODO: [audit] int -> uint needs range check
		relativeOffset = uint64(offset) - dotOffsetBase
	}

	entChan, err := directory.Readdir(relativeOffset, 0).ToFuse()
	if err != nil {
		return err, -fuselib.ENOENT
	}

	for ent := range entChan {
		// stat will always be nil on platforms that have ReaddirPlus disabled
		// and is not gauranteed to be filled on those that do
		if ent.Stat != nil {
			ApplyPermissions(writable, &ent.Stat.Mode)
		}

		if !fill(ent.Name, ent.Stat, dotOffsetBase+ent.Offset) {
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

// TODO: same placehold message as ApplyPermissions
// we'll likely replace instances of this with something more sophisticated
func CheckOpenFlagsBasic(writable bool, flags int) (error, int) {
	// NOTE: SUSv7 doesn't include O_APPEND for EROFS; despite this being a write flag
	// we're counting it for now, but may remove this if it causes compatability problems
	const mutableFlags = fuselib.O_WRONLY | fuselib.O_RDWR | fuselib.O_APPEND | fuselib.O_CREAT | fuselib.O_TRUNC
	if flags&mutableFlags != 0 && !writable {
		return errors.New("write flags provided for read only system"), -fuselib.EROFS
	}
	return nil, OperationSuccess
}
