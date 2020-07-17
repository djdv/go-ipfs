//+build !nofuse

package fuse_test

import (
	"context"
	"testing"

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager"

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/fuse"
	"github.com/ipfs/go-ipfs/filesystem"
	gomfs "github.com/ipfs/go-mfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

func testIPFS(ctx context.Context, t *testing.T, testEnv envData, core coreiface.CoreAPI, filesRoot *gomfs.Root) {
	initChan := make(manager.InitSignal)

	for sysIndex, system := range []struct {
		filesystem.ID
		filesystem.Interface
		readonly bool
	}{
		{ID: filesystem.IPFS},
		{ID: filesystem.IPNS},
		{ID: filesystem.Files},
		{ID: filesystem.PinFS},
		{ID: filesystem.KeyFS},
	} {
		nodeFS, err := manager.NewFileSystem(ctx, system.ID, core, filesRoot)
		if err != nil {
			t.Fatal(err)
		}

		//hostFS := fuse.HostAttacher(ctx, nodeFS)
		hostFS = nil // FIXME: need to use manager.NewHostAttacher(api, fs)

		go hostFS.Init()
		for err := range initChan {
			if err != nil {
				t.Fatalf("subsystem init failed:%s\n", err)
			}
		}

	}

	fs := fuse.NewHostInterface(ctx, core, filesystem.IPFS,
		fuse.WithInitSignal(initChan),
		// WithResourceLock(nodeBinding.IPFSCore), //TODO
	)

	t.Run("Directory operations", func(t *testing.T) { testDirectories(t, testEnv, fs) })
	t.Run("File operations", func(t *testing.T) { testFiles(t, testEnv, core, fs) })

	fs.Destroy()
}
