package mount9p

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	gopath "path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/hugelgupf/p9/fsimpl/localfs"
	"github.com/hugelgupf/p9/p9"
	config "github.com/ipfs/go-ipfs-config"
	serialize "github.com/ipfs/go-ipfs-config/serialize"
	"github.com/ipfs/go-ipfs/core"
	"github.com/ipfs/go-ipfs/core/coreapi"
	"github.com/ipfs/go-ipfs/mount/providers/9P/filesystems/ipfs"
	"github.com/ipfs/go-ipfs/mount/providers/9P/filesystems/ipns"
	"github.com/ipfs/go-ipfs/mount/providers/9P/filesystems/overlay"
	"github.com/ipfs/go-ipfs/mount/providers/9P/filesystems/pinfs"
	"github.com/ipfs/go-ipfs/plugin"
	"github.com/ipfs/go-ipfs/plugin/loader"
	"github.com/ipfs/go-ipfs/repo/common"
	"github.com/ipfs/go-ipfs/repo/fsrepo"
	"github.com/ipfs/go-unixfs/hamt"
	uio "github.com/ipfs/go-unixfs/io"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
)

// TODO: [port from plugin] salvage fs tests; discard plugin tests

var (
	attrMaskIPFSTest = p9.AttrMask{
		Mode: true,
		Size: true,
	}

	rootSubsystems = p9.Dirents{
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
	origPath := os.Getenv("IPFS_PATH")

	repoDir, err := ioutil.TempDir("", "ipfs-fs")
	if err != nil {
		t.Logf("Failed to create repo directory: %s\n", err)
		t.FailNow()
	}

	if err = os.Setenv("IPFS_PATH", repoDir); err != nil {
		t.Logf("Failed to set IPFS_PATH: %s\n", err)
		t.FailNow()
	}

	defer func() {
		if err = os.RemoveAll(repoDir); err != nil {
			t.Logf("Failed to remove test IPFS_PATH: %s\n", err)
			t.Fail()
		}
		if err = os.Setenv("IPFS_PATH", origPath); err != nil {
			t.Logf("Failed to reset IPFS_PATH: %s\n", err)
			t.Fail()
		}
	}()

	ctx := context.TODO()
	node, err := testInitNode(ctx, t)
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
	t.Run("Plugin", func(t *testing.T) { testPlugin(t, node) })

	if err = node.Close(); err != nil {
		t.Error(err)
		t.FailNow()
	}
}

func testPlugin(t *testing.T, node *core.IpfsNode) {
	repoPath, err := config.PathRoot()
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	pluginEnv := &plugin.Environment{Repo: repoPath}
	defaultCfg := defaultConfig()

	for _, pair := range []struct {
		string
		*fsPluginConfig
	}{
		{"none", nil},
		{"default", defaultCfg},
		{"additional", &fsPluginConfig{
			Service: map[string]string{
				defaultService: fmt.Sprintf("/unix/${%s}/%s", tmplHome, sockName),
				"fuse":         "/mnt/",
				"projfs":       `\\someNamespace\`,
				"iokit":        `/Some/Elegant/Path`,
			},
		},
		},
	} {
		pluginEnv.Config = pair.fsPluginConfig
		t.Run(fmt.Sprintf("Config %s", pair.string), func(t *testing.T) { testPluginInit(t, pluginEnv) })
	}

	t.Run("Config malformed 1", func(t *testing.T) {
		module := new(FileSystemPlugin)
		pluginEnv.Config = 42
		if err := module.Init(pluginEnv); err == nil {
			t.Error("Init succeeded with malformed config")
			if err = module.Close(); err != nil {
				t.Error("malformed close succeeded")
			} else {
				t.Error(err)
			}
			t.Fail()
		}
	})

	t.Run("Config malformed 2", func(t *testing.T) {
		module := new(FileSystemPlugin)
		pluginEnv.Config = map[string]int{defaultService: 42}
		if err := module.Init(pluginEnv); err == nil {
			t.Error("Init succeeded with malformed config")
			if err = module.Close(); err != nil {
				t.Error("malformed close succeeded")
			} else {
				t.Error(err)
			}
			t.Fail()
		}
	})

	t.Run("Relative repo path", func(t *testing.T) {
		cwd, err := os.Getwd()
		if err != nil {
			t.Error(err)
			t.Fail()
		}

		relPath, err := filepath.Rel(cwd, repoPath)
		if err != nil {
			t.Error(err)
			t.Fail()
		}

		module := new(FileSystemPlugin)
		pluginEnv := &plugin.Environment{Repo: relPath, Config: defaultCfg}
		if err := module.Init(pluginEnv); err != nil {
			t.Error(err)
			t.Fail()
		}

		if err = module.Close(); err != nil {
			t.Logf("plugin isn't busy, but it can't close: %s", err)
			t.Fail()
		}
	})

	t.Run("long Unix Domain Socket paths", func(t *testing.T) {
		const padding = "SUN"

		socketPath, err := ioutil.TempDir(".", "socket-test")
		if err != nil {
			t.Logf("Failed to create socket directory: %s\n", err)
			t.Fail()
		}

		defer os.RemoveAll(socketPath)
		socketPath = filepath.ToSlash(socketPath)

		// we need to append at least 2 bytes to test
		if sLen := len(socketPath); sLen >= sun_path_len || sLen == sun_path_len-1 {
			t.Skip("temporary directory is already beyond socket length limit, skipping")
		}

		// use a temporary socket path
		var b strings.Builder
		b.WriteString(socketPath)

		// seperate socket file from the rest of the path (NOTE: maddr target not native path)
		b.WriteRune('/')

		// pad path length to original max length
		for i := 0; b.Len() != sun_path_len; i++ {
			b.WriteByte(padding[i%len(padding)])
		}

		module := new(FileSystemPlugin)
		pluginEnv := &plugin.Environment{Repo: repoPath, Config: &fsPluginConfig{map[string]string{
			defaultService: fmt.Sprintf("/unix/%s", b.String()),
		}}}
		if err := module.Init(pluginEnv); err != nil {
			t.Error(err)
			t.Fail()
		}

		if err = module.Start(node); err != nil {
			t.Log("OS does not support long UDS targets")
		} else {
			t.Log("OS does support long UDS targets")
		}

		if err = module.Close(); err != nil {
			t.Logf("plugin isn't busy, but it can't close: %s", err)
			t.Fail()
		}
	})

	t.Run("remove existing Unix Domain Socket", func(t *testing.T) {
		cwd, err := os.Getwd()
		if err != nil {
			t.Error(err)
			t.Fail()
		}

		target := filepath.Join(cwd, "sock")
		f, err := os.Create(target)
		if err != nil {
			t.Error(err)
			t.Fail()
		}

		if err = f.Close(); err != nil {
			t.Error(err)
			t.Fail()
		}

		module := new(FileSystemPlugin)
		pluginEnv := &plugin.Environment{Repo: repoPath, Config: &fsPluginConfig{map[string]string{
			defaultService: fmt.Sprintf("/unix/%s", filepath.ToSlash(target)),
		}}}
		if err := module.Init(pluginEnv); err != nil {
			t.Error(err)
			t.Fail()
		}

		if err = module.Start(node); err != nil {
			t.Log("OS does not support long UDS targets")
		} else {
			t.Log("OS does support long UDS targets")
		}

		if err = module.Close(); err != nil {
			t.Logf("plugin isn't busy, but it can't close: %s", err)
			t.Fail()
		}
	})

	pluginEnv = &plugin.Environment{Repo: repoPath, Config: defaultCfg}
	t.Run("Execution", func(t *testing.T) { testPluginExecution(t, pluginEnv, node) })
}

func testPluginInit(t *testing.T, pluginEnv *plugin.Environment) {
	module := new(FileSystemPlugin)

	if err := module.Init(pluginEnv); err != nil {
		t.Error(err)
		t.FailNow()
	}

	// shouldn't panic
	_ = module.Name()
	_ = module.Version()

	if err := module.Close(); err != nil {
		t.Error(err)
		t.FailNow()
	}
}

func testPluginExecution(t *testing.T, pluginEnv *plugin.Environment, node *core.IpfsNode) {
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
	t.Run("Baseline", func(t *testing.T) { baseline(ctx, t, core, overlay.Attacher) })

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
	t.Run("Baseline", func(t *testing.T) { baseline(ctx, t, core, pinfs.Attacher) })

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
	t.Run("Baseline", func(t *testing.T) { baseline(ctx, t, core, ipfs.Attacher) })
	t.Run("Extra formats", func(t *testing.T) { testIpfsExtra(ctx, t, core) })

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
func testIpfsExtra(ctx context.Context, t *testing.T, core coreiface.CoreAPI) {
	t.Run("Sharded", func(t *testing.T) { testShard(ctx, t, core) })
}

func testIPNS(ctx context.Context, t *testing.T, core coreiface.CoreAPI) {
	t.Run("Baseline", func(t *testing.T) { baseline(ctx, t, core, ipns.Attacher) })
}

func testMFS(ctx context.Context, t *testing.T, core coreiface.CoreAPI) {
	//TODO: init root CID
	//t.Run("Baseline", func(t *testing.T) { baseline(ctx, t, core, fsnodes.MFSAttacher) })
}

func testShard(ctx context.Context, t *testing.T, core coreiface.CoreAPI) {
	entryCount := 32
	// create the sharded structure with an arbitrary ammount of nodes
	shard, err := hamt.NewShard(core.Dag(), entryCount)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	// just add ourself to ourself as data
	subNode, err := uio.NewDirectory(core.Dag()).GetNode()
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	for i := 0; i != entryCount; i++ {
		if err = shard.Set(ctx, strconv.Itoa(i), subNode); err != nil {
			t.Error(err)
			t.FailNow()
		}

		subNode, err = shard.Node()
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
	}

	if err = core.Dag().Add(ctx, subNode); err != nil {
		t.Error(err)
		t.FailNow()
	}

	rootRef, err := ipfs.Attacher(ctx, core).Attach()
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	_, shardedFile, err := rootRef.Walk([]string{subNode.Cid().String()})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if _, _, err = shardedFile.Open(p9.ReadOnly); err != nil {
		t.Error(err)
		t.FailNow()
	}

	ents, err := shardedFile.Readdir(0, uint32(entryCount))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	if len(ents) != entryCount {
		t.Errorf("entry count mismatch for sharded directory")
		t.FailNow()
	}

	if err = shardedFile.Close(); err != nil {
		t.Error(err)
		t.FailNow()
	}

	if err = rootRef.Close(); err != nil {
		t.Error(err)
		t.FailNow()
	}
}

func loadConfigFile() (*fsPluginConfig, error) {
	confPath, err := config.Filename(config.DefaultConfigFile)
	if err != nil {
		return nil, err
	}

	var mapConf map[string]interface{}
	if err := serialize.ReadConfigFile(confPath, &mapConf); err != nil {
		return nil, err
	}

	genericObj, err := common.MapGetKV(mapConf, selectorBase)
	if err != nil {
		return nil, err
	}

	typedConfig, ok := genericObj.(fsPluginConfig)
	if !ok {
		return nil, fmt.Errorf("config was parsed but type does not match expected:")
	}
	return &typedConfig, nil
}

// TODO: [anyone] remove overlap when node constructor refactor is done
func testInitNode(ctx context.Context, t *testing.T) (*core.IpfsNode, error) {
	repoPath, err := config.PathRoot()
	if err != nil {
		t.Logf("Failed to find suitable IPFS repo path: %s\n", err)
		t.FailNow()
		return nil, err
	}

	if err := setupPlugins(repoPath); err != nil {
		t.Logf("Failed to initalize IPFS node plugins: %s\n", err)
		t.FailNow()
		return nil, err
	}

	conf, err := config.Init(ioutil.Discard, 2048)
	if err != nil {
		t.Logf("Failed to construct IPFS node config: %s\n", err)
		t.FailNow()
		return nil, err
	}

	if err := fsrepo.Init(repoPath, conf); err != nil {
		t.Logf("Failed to construct IPFS node repo: %s\n", err)
		t.FailNow()
		return nil, err
	}

	repo, err := fsrepo.Open(repoPath)
	if err := fsrepo.Init(repoPath, conf); err != nil {
		t.Logf("Failed to open newly initalized IPFS repo: %s\n", err)
		t.FailNow()
		return nil, err
	}

	return core.NewNode(ctx, &core.BuildCfg{
		Online:                      false,
		Permanent:                   false,
		DisableEncryptedConnections: true,
		Repo:                        repo,
	})
}

func setupPlugins(path string) error {
	// Load plugins. This will skip the repo if not available.
	plugins, err := loader.NewPluginLoader(filepath.Join(path, "plugins"))
	if err != nil {
		return fmt.Errorf("error loading plugins: %s", err)
	}

	if err := plugins.Initialize(); err != nil {
		return fmt.Errorf("error initializing plugins: %s", err)
	}

	if err := plugins.Inject(); err != nil {
		return fmt.Errorf("error initializing plugins: %s", err)
	}

	return nil
}
