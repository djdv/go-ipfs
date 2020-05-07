package ipfscore

import (
	"context"
	gopath "path"
	"testing"

	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	testutil "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/internal/testutils"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

type fileHandle = uint64

func TestAll(t *testing.T) {
	env, iEnv, node, core, unwind := testutil.GenerateTestEnv(t)
	defer node.Close()
	t.Cleanup(unwind)

	ctx := context.TODO()

	t.Run("IPFS", func(t *testing.T) { testIPFS(ctx, t, env, iEnv, core) })
	// TODO
	//t.Run("IPNS", func(t *testing.T) { testIPNS(ctx, t, env, iEnv, core) })
}

func testIPFS(ctx context.Context, t *testing.T, env string, coreEnv corepath.Resolved, core coreiface.CoreAPI) {

	initChan := make(fusecom.InitSignal)
	fs := NewFileSystem(ctx, core,
		WithNamespace(mountinter.NamespaceIPFS),
		WithCommon(
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
