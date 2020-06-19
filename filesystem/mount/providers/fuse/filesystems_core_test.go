package fuse_test

import (
	"context"
	"testing"

	mountinter "github.com/ipfs/go-ipfs/filesystem/mount"
	fuse "github.com/ipfs/go-ipfs/filesystem/mount/providers/fuse"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

func testIPFS(ctx context.Context, t *testing.T, testEnv envData, core coreiface.CoreAPI) {
	initChan := make(fuse.InitSignal)
	fs := fuse.NewCoreFileSystem(ctx, core, mountinter.NamespaceIPFS,
		fuse.WithInitSignal(initChan),
		// WithResourceLock(fs.IPFSCore), //TODO
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
