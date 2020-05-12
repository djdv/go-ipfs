package fusecommon

import (
	"context"
	"errors"
	"fmt"
	"io"
	gopath "path"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

type fillFunc func(name string, stat *fuselib.Stat_t, ofst int64) bool
type errno = int

func FillDir(ctx context.Context, directory transform.Directory, writable bool, fill fillFunc, offset int64) (error, int) {
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

	const dotOffsetBase = 2 // dot offset ends; stream index 0 begins

	var relativeOffset uint64 // offset used for input, adjusting for dots if any

	switch offset {
	case 0:
		if !fill(".", nil, 1) {
			return nil, OperationSuccess
		}
		if !fill("..", nil, 2) {
			return nil, OperationSuccess
		}
	case 1:
		if !fill("..", nil, 2) {
			return nil, OperationSuccess
		}
	case dotOffsetBase: // do nothing; relativeOffset stays at 0
	default:
		// adjust offset value from dots offset range -> stream offset range
		// TODO: [audit] int -> uint needs range check
		relativeOffset = uint64(offset) - dotOffsetBase
	}

	// only reset stream when the offset is absolute 0
	// relative 0 should not reset underlying stream, but instead return the 0th element + [...]
	if offset != 0 && relativeOffset == 0 {
		if stream, ok := directory.(transform.DirectoryStream); ok {
			stream.DontReset()
		}
	}

	readCtx, cancel := context.WithCancel(ctx)
	defer func() { cancel() }()
	entChan, err := directory.Readdir(readCtx, relativeOffset).ToFuse()
	if err != nil {
		return err, -fuselib.ENOENT
	}

	for ent := range entChan {
		// stat will always be nil on platforms that have ReaddirPlus disabled
		// and is not gauranteed to be filled on those that do
		if ent.Stat != nil {
			ApplyPermissions(writable, &ent.Stat.Mode)
		}

		if !fill(ent.Name, ent.Stat, ent.Offset+dotOffsetBase) {
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
func CheckOpenFlagsBasic(writable bool, flags int) (error, errno) {
	// NOTE: SUSv7 doesn't include O_APPEND for EROFS; despite this being a write flag
	// we're counting it for now, but may remove this if it causes compatability problems
	const mutableFlags = fuselib.O_WRONLY | fuselib.O_RDWR | fuselib.O_APPEND | fuselib.O_CREAT | fuselib.O_TRUNC
	if flags&mutableFlags != 0 && !writable {
		return errors.New("write flags provided for read only system"), -fuselib.EROFS
	}
	return nil, OperationSuccess
}

func CheckOpenPathBasic(path string) (error, int) {
	switch path {
	case "":
		return fuselib.Error(-fuselib.ENOENT), -fuselib.ENOENT
	case "/":
		return fuselib.Error(-fuselib.EISDIR), -fuselib.EISDIR
	default:
		return nil, OperationSuccess
	}
}

// TODO: these are backwards, convention is that error is last
func ReleaseFile(table FileTable, handle uint64) (error, errno) {
	file, err := table.Get(handle)
	if err != nil {
		return err, -fuselib.EBADF
	}

	// SUSv7 `close` (parphrased)
	// if errors are encountered, the result of the handle is unspecified
	// for us specifically, we'll remove the handle regardless of its close return

	if err := table.Remove(handle); err != nil {
		// TODO: if the error is not found we need to panic or return a severe error
		// this should not be possible
		return err, -fuselib.EBADF
	}

	return file.Close(), OperationSuccess
}

func ReleaseDir(table DirectoryTable, handle uint64) (error, errno) {
	dir, err := table.Get(handle)
	if err != nil {
		return err, -fuselib.EBADF
	}

	if err := table.Remove(handle); err != nil {
		// TODO: if the error is not found we need to panic or return a severe error
		// this should not be possible
		return err, -fuselib.EBADF
	}

	// NOTE: even if close fails, we return system success
	// the relevant standard errors do not apply here; `releasedir` is not expected to fail
	// the handle was valid [EBADF], and we didn't get interupted by the system [EINTR]
	// if the returned error is not nil, it's an implementation fault that needs to be amended
	// in the directory interface implementation returned to us from the table
	return dir.Close(), OperationSuccess
}

func ReadFile(file transform.File, buff []byte, ofst int64) (error, errno) {
	if len(buff) == 0 {
		return nil, 0
	}

	if ofst < 0 {
		return fmt.Errorf("invalid offset %d", ofst), -fuselib.EINVAL
	}

	if fileBound, err := file.Size(); err == nil {
		if ofst >= fileBound {
			return nil, 0 // POSIX expects this
		}
	}

	if ofst != 0 {
		_, err := file.Seek(ofst, io.SeekStart)
		if err != nil {
			return fmt.Errorf("offset seek error: %s", err), -fuselib.EIO
		}
	}

	buffLen := len(buff)
	readBytes, err := file.Read(buff)
	if err != nil && err != io.EOF {
		return fmt.Errorf("Read - error: %s", err), -fuselib.EIO
	}

	// io.Reader:
	// Even if Read returns n < len(p), it may use all of p as scratch space during the call.
	// we want to assure these are 0'd
	if readBytes < buffLen {
		for i := readBytes; i != buffLen; i++ {
			buff[i] = 0
		}
	}

	// EOF will be returned if it was provided
	return err, readBytes
}
