package filesystem

import (
	"context"
	"errors"
	"fmt"
	"os"
	gopath "path"
	"sort"
	"testing"

	"github.com/hugelgupf/p9/localfs"
	"github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-ipfs/core"
	"github.com/ipfs/go-ipfs/core/coreapi"
	"github.com/ipfs/go-ipfs/plugin"
	"github.com/ipfs/go-ipfs/plugin/plugins/filesystem/filesystems/ipfs"
	"github.com/ipfs/go-ipfs/plugin/plugins/filesystem/filesystems/ipns"
	"github.com/ipfs/go-ipfs/plugin/plugins/filesystem/filesystems/overlay"
	"github.com/ipfs/go-ipfs/plugin/plugins/filesystem/filesystems/pinfs"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

var (
	attrMaskIPFSTest = p9.AttrMask{
		Mode: true,
		Size: true,
	}

	rootSubsystems = []p9.Dirent{
		{
			Name:   "ipfs",
			Offset: 1,
			Type:   p9.TypeDir,
			QID: p9.QID{
				Type: p9.TypeDir,
			},
		}, {
			Name:   "ipns",
			Offset: 2,
			Type:   p9.TypeDir,
			QID: p9.QID{
				Type: p9.TypeDir,
			},
		},
	}
)

func TestAll(t *testing.T) {
	ctx := context.TODO()

	node, err := core.NewNode(ctx, &core.BuildCfg{
		Online:                      false,
		Permanent:                   false,
		DisableEncryptedConnections: true,
	})

	if err != nil {
		t.Logf("Failed to construct IPFS node: %s\n", err)
		t.FailNow()
	}

	core, err := coreapi.NewCoreAPI(node)
	if err != nil {
		t.Logf("Failed to construct CoreAPI: %s\n", err)
		t.FailNow()
	}

	t.Run("RootFS", func(t *testing.T) { testRootFS(ctx, t, core) })
	t.Run("PinFS", func(t *testing.T) { testPinFS(ctx, t, core) })
	t.Run("IPFS", func(t *testing.T) { testIPFS(ctx, t, core) })
	t.Run("MFS", func(t *testing.T) { testMFS(ctx, t, core) })
	t.Run("IPNS", func(t *testing.T) { testIPNS(ctx, t, core) })

	pluginEnv := &plugin.Environment{Config: defaultConfig()}
	t.Run("Plugin", func(t *testing.T) { testPlugin(t, pluginEnv, node) })
}

func testPlugin(t *testing.T, pluginEnv *plugin.Environment, node *core.IpfsNode) {
	// NOTE: all restrictive comments are in relation to our plugin, not all plugins
	var (
		module FileSystemPlugin
		err    error
	)

	// close and start before init are NOT allowed
	if err = module.Close(); err == nil {
		t.Logf("plugin was not initialized but Close succeeded")
		t.FailNow()
		// also should not hang
	}
	if err = module.Start(node); err == nil {
		t.Logf("plugin was not initialized but Start succeeded")
		t.FailNow()
		// also should not hang
	}

	// initialize the module
	if err = module.Init(pluginEnv); err != nil {
		t.Logf("Plugin couldn't be initialized: %s", err)
		t.FailNow()
	}

	// double init is NOT allowed
	if err = module.Init(pluginEnv); err == nil {
		t.Logf("init isn't intended to succeed twice")
		t.FailNow()
	}

	// close before start is allowed
	if err = module.Close(); err != nil {
		t.Logf("plugin isn't busy, but it can't close: %s", err)
		t.FailNow()
		// also should not hang
	}

	// double close is allowed
	if err = module.Close(); err != nil {
		t.Logf("plugin couldn't close twice: %s", err)
		t.FailNow()
	}

	// start the module
	if err = module.Start(node); err != nil {
		t.Logf("module could not start: %s", err)
		t.FailNow()
	}

	// double start is NOT allowed
	if err = module.Start(node); err == nil {
		t.Logf("module is intended to be exclusive but was allowed to start twice")
		t.FailNow()
	}

	// actual close
	if err = module.Close(); err != nil {
		t.Logf("plugin isn't busy, but it can't close: %s", err)
		t.FailNow()
	}

	// another redundant close
	if err = module.Close(); err != nil {
		t.Logf("plugin isn't busy, but it can't close: %s", err)
		t.FailNow()
		// also should not hang
	}
}

func testRootFS(ctx context.Context, t *testing.T, core coreiface.CoreAPI) {
	t.Run("Baseline", func(t *testing.T) { baseLine(ctx, t, core, overlay.Attacher) })

	rootRef, err := overlay.Attacher(ctx, core).Attach()
	if err != nil {
		t.Logf("Baseline test passed but attach failed: %s\n", err)
		t.FailNow()
	}
	_, root, err := rootRef.Walk(nil)
	if err != nil {
		t.Logf("Baseline test passed but walk failed: %s\n", err)
		t.FailNow()
	}

	t.Run("Root directory entries", func(t *testing.T) { testRootDir(ctx, t, root) })
}

func testRootDir(ctx context.Context, t *testing.T, root p9.File) {
	root.Open(p9.ReadOnly)

	ents, err := root.Readdir(0, uint32(len(rootSubsystems)))
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	if _, err = root.Readdir(uint64(len(ents)), ^uint32(0)); err != nil {
		t.Log(errors.New("entry count mismatch"))
		t.FailNow()
	}

	for i, ent := range ents {
		// TODO: for now we trust the QID from the server
		// we should generate these paths separately during init
		rootSubsystems[i].QID.Path = ent.QID.Path

		if ent != rootSubsystems[i] {
			t.Log(fmt.Errorf("ent %v != expected %v", ent, rootSubsystems[i]))
			t.FailNow()
		}
	}
}

func testPinFS(ctx context.Context, t *testing.T, core coreiface.CoreAPI) {
	t.Run("Baseline", func(t *testing.T) { baseLine(ctx, t, core, pinfs.Attacher) })

	pinRoot, err := pinfs.Attacher(ctx, core).Attach()
	if err != nil {
		t.Logf("Failed to attach to 9P Pin resource: %s\n", err)
		t.FailNow()
	}

	same := func(base, target []string) bool {
		if len(base) != len(target) {
			return false
		}
		sort.Strings(base)
		sort.Strings(target)

		for i := len(base) - 1; i >= 0; i-- {
			if target[i] != base[i] {
				return false
			}
		}
		return true
	}

	shallowCompare := func() {
		basePins, err := pinNames(ctx, core)
		if err != nil {
			t.Logf("Failed to list IPFS pins: %s\n", err)
			t.FailNow()
		}
		p9Pins, err := p9PinNames(pinRoot)
		if err != nil {
			t.Logf("Failed to list 9P pins: %s\n", err)
			t.FailNow()
		}

		if !same(basePins, p9Pins) {
			t.Logf("Pinsets differ\ncore: %v\n9P: %v\n", basePins, p9Pins)
			t.FailNow()
		}
	}

	//test default (likely empty) test repo pins
	shallowCompare()

	// test modifying pinset +1; initEnv pins its IPFS environment
	env, _, err := initEnv(ctx, core)
	if err != nil {
		t.Logf("Failed to construct IPFS test environment: %s\n", err)
		t.FailNow()
	}
	defer os.RemoveAll(env)
	shallowCompare()

	// test modifying pinset +1 again; generate garbage and pin it
	if err := generateGarbage(env); err != nil {
		t.Logf("Failed to generate test data: %s\n", err)
		t.FailNow()
	}
	if _, err = pinAddDir(ctx, core, env); err != nil {
		t.Logf("Failed to add directory to IPFS: %s\n", err)
		t.FailNow()
	}
	shallowCompare()
}

func testIPFS(ctx context.Context, t *testing.T, core coreiface.CoreAPI) {
	t.Run("Baseline", func(t *testing.T) { baseLine(ctx, t, core, ipfs.Attacher) })

	rootRef, err := ipfs.Attacher(ctx, core).Attach()
	if err != nil {
		t.Logf("Baseline test passed but attach failed: %s\n", err)
		t.FailNow()
	}

	env, iEnv, err := initEnv(ctx, core)
	if err != nil {
		t.Logf("Failed to construct IPFS test environment: %s\n", err)
		t.FailNow()
	}
	defer os.RemoveAll(env)

	localEnv, err := localfs.Attacher(env).Attach()
	if err != nil {
		t.Logf("Failed to attach to local resource %q: %s\n", env, err)
		t.FailNow()
	}

	_, ipfsEnv, err := rootRef.Walk([]string{gopath.Base(iEnv.String())})
	if err != nil {
		t.Logf("Failed to walk to IPFS test environment: %s\n", err)
		t.FailNow()
	}
	_, envClone, err := ipfsEnv.Walk(nil)
	if err != nil {
		t.Logf("Failed to clone IPFS environment handle: %s\n", err)
		t.FailNow()
	}

	testCompareTreeAttrs(t, localEnv, ipfsEnv)

	// test readdir bounds
	//TODO: compare against a table, not just lengths
	_, _, err = envClone.Open(p9.ReadOnly)
	if err != nil {
		t.Logf("Failed to open IPFS test directory: %s\n", err)
		t.FailNow()
	}
	ents, err := envClone.Readdir(2, 2) // start at ent 2, return max 2
	if err != nil {
		t.Logf("Failed to read IPFS test directory: %s\n", err)
		t.FailNow()
	}
	if l := len(ents); l == 0 || l > 2 {
		t.Logf("IPFS test directory contents don't match read request: %v\n", ents)
		t.FailNow()
	}
}

func testIPNS(ctx context.Context, t *testing.T, core coreiface.CoreAPI) {
	t.Run("Baseline", func(t *testing.T) { baseLine(ctx, t, core, ipns.Attacher) })
}

func testMFS(ctx context.Context, t *testing.T, core coreiface.CoreAPI) {
	//TODO: init root CID
	//t.Run("Baseline", func(t *testing.T) { baseLine(ctx, t, core, fsnodes.MFSAttacher) })
}
