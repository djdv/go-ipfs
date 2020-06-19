package fuse

import (
	"context"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	"github.com/ipfs/go-ipfs/filesystem/interfaces/ipfscore"
	"github.com/ipfs/go-ipfs/filesystem/interfaces/keyfs"
	"github.com/ipfs/go-ipfs/filesystem/interfaces/mfs"
	"github.com/ipfs/go-ipfs/filesystem/interfaces/pinfs"
	mountinter "github.com/ipfs/go-ipfs/filesystem/mount"
	provcom "github.com/ipfs/go-ipfs/filesystem/mount/providers"
	gomfs "github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

/* overlay
func NewCombinedFileSystem(ctx context.Context, core coreiface.CoreAPI, opts ...CoreSystemOption) fuselib.FileSystemInterface {
}
*/

func NewCoreFileSystem(ctx context.Context, core coreiface.CoreAPI, namespace mountinter.Namespace, opts ...SystemOption) fuselib.FileSystemInterface {
	var logName string // if a log was not provided, we'll provide a more specific default
	switch namespace {
	case mountinter.NamespaceIPFS:
		logName = "fuse/ipfs"
	case mountinter.NamespaceIPNS:
		logName = "fuse/ipns"
	default:
		logName = "fuse/ipld"
	}
	opts = maybeAppendLog(opts, logName)

	settings := parseSystemOptions(opts...)

	fs := &fileSystem{
		intf:     ipfscore.NewInterface(ctx, core, namespace),
		initChan: settings.InitSignal,
		log:      settings.log,
	}

	if provcom.CanReaddirPlus {
		if namespace == mountinter.NamespaceIPFS {
			fs.readdirplusGen = staticStat
		} else {
			fs.readdirplusGen = dynamicStat
		}
	}

	return fs
}

func NewPinFileSystem(ctx context.Context, core coreiface.CoreAPI, opts ...SystemOption) fuselib.FileSystemInterface {
	opts = maybeAppendLog(opts, "fuse/pinfs")
	settings := parseSystemOptions(opts...)

	fs := &fileSystem{
		intf:     pinfs.NewInterface(ctx, core),
		initChan: settings.InitSignal,
		log:      settings.log,
	}

	if provcom.CanReaddirPlus {
		fs.readdirplusGen = staticStat
	}

	return fs
}

func NewKeyFileSystem(ctx context.Context, core coreiface.CoreAPI, opts ...SystemOption) fuselib.FileSystemInterface {
	opts = maybeAppendLog(opts, "fuse/keyfs")
	settings := parseSystemOptions(opts...)

	fs := &fileSystem{
		intf:          keyfs.NewInterface(ctx, core),
		initChan:      settings.InitSignal,
		log:           settings.log,
		filesWritable: true,
	}

	if provcom.CanReaddirPlus {
		fs.readdirplusGen = dynamicStat
	}

	return fs
}

func NewMutableFileSystem(ctx context.Context, mroot *gomfs.Root, opts ...SystemOption) fuselib.FileSystemInterface {
	opts = maybeAppendLog(opts, "fuse/mfs")
	settings := parseSystemOptions(opts...)

	fs := &fileSystem{
		intf:          mfs.NewInterface(ctx, mroot),
		initChan:      settings.InitSignal,
		log:           settings.log,
		filesWritable: true,
	}

	if provcom.CanReaddirPlus {
		fs.readdirplusGen = dynamicStat
	}

	return fs
}
