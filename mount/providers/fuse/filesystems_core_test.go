package mountfuse

import (
	"context"
	"testing"

	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	ipfscore "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/core"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

func testIPFS(ctx context.Context, t *testing.T, testEnv envData, core coreiface.CoreAPI) {

	initChan := make(fusecom.InitSignal)
	fs := ipfscore.NewFileSystem(ctx, core,
		ipfscore.WithNamespace(mountinter.NamespaceIPFS),
		ipfscore.WithCommon(
			fusecom.WithInitSignal(initChan),
			// fusecom.WithResourceLock(fs.IPFSCore), //TODO
		),
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
