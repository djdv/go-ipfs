package p9fsp

import (
	"context"
	"time"

	"github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-ipfs/filesystem/interfaces/ipfscore"
	"github.com/ipfs/go-ipfs/filesystem/interfaces/keyfs"
	"github.com/ipfs/go-ipfs/filesystem/interfaces/mfs"
	"github.com/ipfs/go-ipfs/filesystem/interfaces/pinfs"
	mountinter "github.com/ipfs/go-ipfs/filesystem/mount"
	gomfs "github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

func CoreAttacher(ctx context.Context, core coreiface.CoreAPI, namespace mountinter.Namespace, opts ...AttachOption) p9.Attacher {
	var logName string // if a log was not provided, we'll provide a more specific default
	switch namespace {
	case mountinter.NamespaceIPFS:
		logName = "9p/ipfs"
	case mountinter.NamespaceIPNS:
		logName = "9p/ipns"
	default:
		logName = "9p/ipld"
	}

	opts = maybeAppendLog(opts, logName)
	settings := parseAttachOptions(opts...)

	fid := &fid{
		intf:     ipfscore.NewInterface(ctx, core, namespace),
		log:      settings.log,
		initTime: time.Now(),
	}

	// TODO: set static attribute on FID (never refresh stats or times)
	// if namespace == IPFS ...

	return fid
}

func PinAttacher(ctx context.Context, core coreiface.CoreAPI, opts ...AttachOption) p9.Attacher {
	opts = maybeAppendLog(opts, "9p/pinfs")
	settings := parseAttachOptions(opts...)

	fid := &fid{
		intf:     pinfs.NewInterface(ctx, core),
		log:      settings.log,
		initTime: time.Now(),
	}

	return fid
}

func KeyAttacher(ctx context.Context, core coreiface.CoreAPI, opts ...AttachOption) p9.Attacher {
	opts = maybeAppendLog(opts, "9p/keyfs")
	settings := parseAttachOptions(opts...)

	fid := &fid{
		intf:          keyfs.NewInterface(ctx, core),
		log:           settings.log,
		initTime:      time.Now(),
		filesWritable: true,
	}

	return fid
}

func MutableAttacher(ctx context.Context, mroot *gomfs.Root, opts ...AttachOption) p9.Attacher {
	opts = maybeAppendLog(opts, "9p/mfs")
	settings := parseAttachOptions(opts...)

	fid := &fid{
		intf:          mfs.NewInterface(ctx, mroot),
		log:           settings.log,
		initTime:      time.Now(),
		filesWritable: true,
	}

	return fid
}
