package p9fsp

import (
	"time"

	"github.com/hugelgupf/p9/p9"
	ninelib "github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-ipfs/filesystem"
	transform "github.com/ipfs/go-ipfs/filesystem"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

const ( // pedantic POSIX stuff
	S_IROTH p9.FileMode = p9.Read
	S_IWOTH             = p9.Write
	S_IXOTH             = p9.Exec

	S_IRGRP = S_IROTH << 3
	S_IWGRP = S_IWOTH << 3
	S_IXGRP = S_IXOTH << 3

	S_IRUSR = S_IRGRP << 3
	S_IWUSR = S_IWGRP << 3
	S_IXUSR = S_IXGRP << 3

	S_IRWXO = S_IROTH | S_IWOTH | S_IXOTH
	S_IRWXG = S_IRGRP | S_IWGRP | S_IXGRP
	S_IRWXU = S_IRUSR | S_IWUSR | S_IXUSR

	IRWXA = S_IRWXU | S_IRWXG | S_IRWXO            // 0777
	IRXA  = IRWXA &^ (S_IWUSR | S_IWGRP | S_IWOTH) // 0555
)

type timeGroup struct {
	atime, mtime, ctime, btime time.Time
}

type idGroup struct {
	uid ninelib.UID
	gid ninelib.GID
}

func coreTypeTo9PType(ct coreiface.FileType) p9.FileMode {
	switch ct {
	case coreiface.TDirectory:
		return p9.ModeDirectory
	case coreiface.TSymlink:
		return p9.ModeSymlink
	case coreiface.TFile:
		return p9.ModeRegular
	default:
		return p9.FileMode(0)
	}
}

func ioFlagsFrom9P(nineFlagsAmusementPark ninelib.OpenFlags) transform.IOFlags {
	switch nineFlagsAmusementPark.Mode() {
	case ninelib.ReadOnly:
		return transform.IOReadOnly
	case ninelib.WriteOnly:
		return transform.IOWriteOnly
	case ninelib.ReadWrite:
		return transform.IOReadWrite
	default:
		return transform.IOFlags(0)
	}
}

func requestFrom9P(req p9.AttrMask) filesystem.IPFSStatRequest {
	var iReq filesystem.IPFSStatRequest
	if req.Mode {
		iReq.Type = true
	}
	if req.Size {
		iReq.Size = true
	}
	if iReq.Blocks {
		iReq.Blocks = true
	}
	return iReq
}

func filledFromCore(coreFilled filesystem.IPFSStatRequest) (nineFilled p9.AttrMask) {
	nineFilled.Mode = coreFilled.Type
	nineFilled.Size = coreFilled.Size
	nineFilled.Blocks = coreFilled.Blocks
	return
}

func attrFromCore(cs *filesystem.IPFSStat) p9.Attr {
	// TODO [safety] we should probably panic if the uint64 source values exceed int64 positive range
	return p9.Attr{
		Mode:      coreTypeTo9PType(cs.FileType),
		Size:      cs.Size,
		BlockSize: cs.BlockSize,
		Blocks:    cs.Blocks,
	}
}

func applyCommonsToAttr(attr *ninelib.Attr, writable bool, tg timeGroup, ids idGroup) {
	attr.ATimeSeconds, attr.ATimeNanoSeconds = uint64(tg.atime.Unix()), uint64(tg.atime.UnixNano())
	attr.MTimeSeconds, attr.MTimeNanoSeconds = uint64(tg.mtime.Unix()), uint64(tg.mtime.UnixNano())
	attr.CTimeSeconds, attr.CTimeNanoSeconds = uint64(tg.ctime.Unix()), uint64(tg.ctime.UnixNano())
	attr.BTimeSeconds, attr.BTimeNanoSeconds = uint64(tg.btime.Unix()), uint64(tg.btime.UnixNano())

	attr.UID, attr.GID = ids.uid, ids.gid

	// TODO: [review] 9P permissions may have subtle differences
	// specifically re-read the section on the creation mask used for dirents
	// something about inheritance
	if writable {
		attr.Mode |= IRWXA &^ (S_IWOTH | S_IXOTH) // |0774
	} else {
		attr.Mode |= IRXA &^ (S_IXOTH) // |0554
	}
}
