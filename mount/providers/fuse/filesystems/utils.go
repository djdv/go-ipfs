package fusecommon

import (
	"context"
	"errors"
	"fmt"
	"io"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
)

type fuseFillFunc func(name string, stat *fuselib.Stat_t, ofst int64) bool
type errNo = int

// DirectoryPlus is a compatible directory, containing a method to stat it's children
// (useful for conditionally handling FUSE's readdir plus feature via a type assertion)
type DirectoryPlus struct {
	transform.Directory
	StatFunc
}
type StatFunc func(name string) *fuselib.Stat_t

// TODO: return values are backwards, go errors should come last
func FillDir(ctx context.Context, directory transform.Directory, fill fuseFillFunc, offset int64) (error, int) {

	// Offset value 0 has a special meaning in FUSE (see: FUSE's `readdir` docs)
	// so all returned offsets values from us are expected to be 0>
	// FillDir expects the input directory to follow this convention, and supply us with offsets 0>
	// to avoid overlap, or range requirements
	// we sum our local (dot) offset with the entry's offset to get a value suitable to return
	// and do the inverse to get the directory's input offset value (from a value we previously returned)
	// relevant reads: SUSv7 `readdir`, `seekdir`, `telldir`

	// TODO: [POSIX] find out if the lack of dots actually breaks anything
	// SUSv7 says they're optional in `opendir`, however `readdir` explicitly forbids them
	// "If entries for dot or dot-dot exist, one entry shall be returned for dot and one entry shall be returned for dot-dot; otherwise, they shall not be returned."
	// this should likely be a config value somewhere; disabled by default?
	// conf: mount.fuse.enabledots; newFuseProvider(provideropt{dots})
	const dotOffsetBase = 2 // FillDir offset ends; stream index 0 begins

	var relativeOffset uint64 // offset used for input, adjusting for dots if any

	var (
		statFunc StatFunc
		stat     *fuselib.Stat_t
	)

	if dirPlus, ok := directory.(*DirectoryPlus); ok {
		statFunc = dirPlus.StatFunc
	} else {
		statFunc = func(string) *fuselib.Stat_t { return nil }
	}

	switch offset {
	case 0:
		stat = statFunc(".")
		if !fill(".", stat, 1) {
			return nil, OperationSuccess
		}
		fallthrough

	case 1:
		stat = statFunc("..")
		if !fill("..", stat, 2) {
			return nil, OperationSuccess
		}
	case dotOffsetBase:
		// do nothing; relativeOffset stays at 0

	default: // `case (offset > dotOffsetBase):`
		// adjust offset value from FillDir's offset range -> stream's offset range
		// TODO: [audit] int -> uint needs range check
		relativeOffset = uint64(offset) - dotOffsetBase
	}

	// only reset stream when the offset is absolute 0
	// relative 0 should not reset underlying stream, but instead return the 0th element + [...]
	if offset == 0 {
		// TODO: [d21a38b9-e723-4068-ad72-7473b91cc770]
		if err := directory.Reset(); err != nil {
			return err, -fuselib.ENOENT // TODO: [SUS check]: appropriate value?
		}
	}

	readCtx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
	}()

	for ent := range directory.List(readCtx, relativeOffset) {
		if err := ent.Error(); err != nil {
			return err, -fuselib.ENOENT // TODO: just stuck in default error value, should probably be a tErr
		}
		stat = statFunc(ent.Name())
		// TODO: uint <-> int shenanigans
		if !fill(ent.Name(), stat, int64(ent.Offset())+dotOffsetBase) {
			break
		}
	}

	return nil, OperationSuccess
}

type StatTimeGroup struct {
	Atim, Mtim, Ctim, Birthtim fuselib.Timespec
}

type StatIDGroup struct {
	Uid, Gid uint32
	// These are omitted for now (as they're not used by us)
	// but belong in this structure if they become needed
	// Dev, Rdev uint64
}

func ApplyCommonsToStat(stat *fuselib.Stat_t, writable bool, tg StatTimeGroup, ids StatIDGroup) {
	stat.Atim, stat.Mtim, stat.Ctim, stat.Birthtim = tg.Atim, tg.Mtim, tg.Ctim, tg.Birthtim
	stat.Uid, stat.Gid = ids.Uid, ids.Gid

	if writable {
		stat.Mode |= IRWXA &^ (fuselib.S_IWOTH | fuselib.S_IXOTH) // |0774
	} else {
		stat.Mode |= IRXA &^ (fuselib.S_IXOTH) // |0554
	}
}

// TODO: same placehold message as ApplyPermissions
// we'll likely replace instances of this with something more sophisticated
func CheckOpenFlagsBasic(writable bool, flags int) (error, errNo) {
	// NOTE: SUSv7 doesn't include O_APPEND for EROFS; despite this being a write flag
	// we're counting it for now, but may remove this if it causes compatibility problems
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
func ReleaseFile(table FileTable, handle uint64) (error, errNo) {
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

func ReleaseDir(table DirectoryTable, handle uint64) (error, errNo) {
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

// TODO: read+write; we're not accounting for scenarios where the offset is beyond the end of the file
func ReadFile(file transform.File, buff []byte, ofst int64) (error, errNo) {
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

	if _, err := file.Seek(ofst, io.SeekStart); err != nil {
		return err, -fuselib.EIO
	}

	buffLen := len(buff)
	readBytes, err := file.Read(buff)
	if err != nil && err != io.EOF {
		return err, -fuselib.EIO
	}

	// io.Reader:
	// Even if Read returns n < len(p), it may use all of p as scratch space during the call.
	// we want to assure these are 0'd
	if readBytes < buffLen {
		nilZone := buff[readBytes:]
		copy(nilZone, make([]byte, len(nilZone)))
	}

	// EOF will be returned if it was provided
	return err, readBytes
}

func WriteFile(file transform.File, buff []byte, ofst int64) (error, errNo) {
	if len(buff) == 0 {
		return nil, 0
	}

	if ofst < 0 {
		return fmt.Errorf("invalid offset %d", ofst), -fuselib.EINVAL
	}

	/* TODO: test this; it should be handled internally by seek()+write()
	if fileBound, err := file.Size(); err == nil {
		if ofst >= fileBound {
			newEnd := fileBound - (ofst - int64(len(buff)))
			if err := file.Truncate(uint64(newEnd)); err != nil { // pad 0's before our write
				return err, -fuselib.EIO
			}
		}
	}
	*/

	if _, err := file.Seek(ofst, io.SeekStart); err != nil {
		return fmt.Errorf("offset seek error: %s", err), -fuselib.EIO
	}

	wroteBytes, err := file.Write(buff)
	if err != nil {
		return err, -fuselib.EIO
	}

	return nil, wroteBytes
}
