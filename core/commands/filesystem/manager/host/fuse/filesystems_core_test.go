//+build !nofuse

package fuse_test

import (
	"context"
	"testing"

	fsm "github.com/ipfs/go-ipfs/core/commands/filesystem/manager"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/fuse"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

func testIPFS(ctx context.Context, t *testing.T, testEnv envData, core coreiface.CoreAPI) {
	initChan := make(fuse.InitSignal)
	fs := fuse.NewCoreFileSystem(ctx, core, fsm.IPFS,
		fuse.WithInitSignal(initChan),
		// WithResourceLock(fuseInterface.IPFSCore), //TODO
	)

	go fs.Init()
	for err := range initChan {
		if err != nil {
			t.Fatalf("subsystem init failed:%s\n", err)
		}
	}

	t.Run("Directory operations", func(t *testing.T) { testDirectories(t, testEnv, fs) })
	t.Run("File operations", func(t *testing.T) { testFiles(t, testEnv, core, fs) })

	fs.Destroy()
}
