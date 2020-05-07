package mountfuse

import (
	"context"
	gopath "path"
	"testing"

	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	ipfscore "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/core"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

func testIPFS(ctx context.Context, t *testing.T, env string, coreEnv corepath.Resolved, core coreiface.CoreAPI) {

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

	corePath := gopath.Base(coreEnv.String())
	t.Run("Directory operations", func(t *testing.T) { testDirectories(t, env, corePath, fs) })
	t.Run("File operations", func(t *testing.T) { testFiles(t, env, core, fs) })
}
