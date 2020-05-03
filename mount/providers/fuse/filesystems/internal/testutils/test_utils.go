package testutil

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	config "github.com/ipfs/go-ipfs-config"
	files "github.com/ipfs/go-ipfs-files"
	"github.com/ipfs/go-ipfs/core"
	"github.com/ipfs/go-ipfs/core/coreapi"
	"github.com/ipfs/go-ipfs/plugin/loader"
	"github.com/ipfs/go-ipfs/repo/fsrepo"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	coreoptions "github.com/ipfs/interface-go-ipfs-core/options"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

const incantation = "May the bits passing through this device somehow help bring peace to this world"

func testInitNode(ctx context.Context, t *testing.T) (*core.IpfsNode, error) {
	repoPath, err := config.PathRoot()
	if err != nil {
		t.Logf("Failed to find suitable IPFS repo path: %s\n", err)
		t.FailNow()
		return nil, err
	}

	if err := setupPlugins(repoPath); err != nil {
		t.Logf("Failed to initialize IPFS node plugins: %s\n", err)
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

func generateEnvData(ctx context.Context, core coreiface.CoreAPI) (string, corepath.Resolved, error) {
	testDir, err := ioutil.TempDir("", "ipfs-")
	if err != nil {
		return "", nil, err
	}
	if err := os.Chmod(testDir, 0775); err != nil {
		return "", nil, err
	}

	if err = ioutil.WriteFile(filepath.Join(testDir, "empty"),
		[]byte(nil),
		0644); err != nil {
		return "", nil, err
	}

	if err = ioutil.WriteFile(filepath.Join(testDir, "small"),
		[]byte(incantation),
		0644); err != nil {
		return "", nil, err
	}

	if err := generateGarbage(testDir); err != nil {
		return "", nil, err
	}

	testSubDir, err := ioutil.TempDir(testDir, "ipfs-")
	if err != nil {
		return "", nil, err
	}
	if err := os.Chmod(testSubDir, 0775); err != nil {
		return "", nil, err
	}

	if err := generateGarbage(testSubDir); err != nil {
		return "", nil, err
	}

	iPath, err := pinAddDir(ctx, core, testDir)
	if err != nil {
		return "", nil, err
	}

	return testDir, iPath, err
}

func pinAddDir(ctx context.Context, core coreiface.CoreAPI, path string) (corepath.Resolved, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	node, err := files.NewSerialFile(path, false, fi)
	if err != nil {
		return nil, err
	}

	iPath, err := core.Unixfs().Add(ctx, node.(files.Directory), coreoptions.Unixfs.Pin(true))
	if err != nil {
		return nil, err
	}
	return iPath, nil
}

func generateGarbage(tempDir string) error {
	randDev := rand.New(rand.NewSource(time.Now().UnixNano()))

	for _, size := range []int{4, 8, 16, 32} {
		buf := make([]byte, size<<(10*2))
		if _, err := randDev.Read(buf); err != nil {
			return err
		}

		name := fmt.Sprintf("%dMiB", size)
		if err := ioutil.WriteFile(filepath.Join(tempDir, name),
			buf,
			0644); err != nil {
			return err
		}
	}

	return nil
}

// TODO: see if we can circumvent import cycle hell and not have to reconstruct the node for each filesystem test
func GenerateTestEnv(t *testing.T) (string, corepath.Resolved, *core.IpfsNode, coreiface.CoreAPI, func()) {
	// environment setup
	origPath := os.Getenv("IPFS_PATH")

	unwindStack := make([]func(), 0)
	unwind := func() {
		for i := len(unwindStack) - 1; i > -1; i-- {
			unwindStack[i]()
		}
	}

	repoDir, err := ioutil.TempDir("", "ipfs-fs")
	if err != nil {
		t.Errorf("Failed to create repo directory: %s\n", err)
	}

	unwindStack = append(unwindStack, func() {
		if err = os.RemoveAll(repoDir); err != nil {
			t.Errorf("Failed to remove test repo directory: %s\n", err)
		}
	})

	if err = os.Setenv("IPFS_PATH", repoDir); err != nil {
		t.Logf("Failed to set IPFS_PATH: %s\n", err)
		unwind()
		t.FailNow()
	}

	unwindStack = append(unwindStack, func() {
		if err = os.Setenv("IPFS_PATH", origPath); err != nil {
			t.Errorf("Failed to reset IPFS_PATH: %s\n", err)
		}
	})

	// node actual
	ctx := context.TODO()
	node, err := testInitNode(ctx, t)
	if err != nil {
		t.Logf("Failed to construct IPFS node: %s\n", err)
		unwind()
		t.FailNow()
	}
	unwindStack = append(unwindStack, func() {
		if err := node.Close(); err != nil {
			t.Errorf("Failed to close node:%s", err)
		}
	})

	core, err := coreapi.NewCoreAPI(node)
	if err != nil {
		t.Logf("Failed to construct CoreAPI: %s\n", err)
		unwind()
		t.FailNow()
	}

	// add data to some local path and to the node
	env, iEnv, err := generateEnvData(ctx, core)
	if err != nil {
		t.Logf("Failed to construct IPFS test environment: %s\n", err)
		unwind()
		t.FailNow()
	}
	unwindStack = append(unwindStack, func() {
		if err := os.RemoveAll(env); err != nil {
			t.Errorf("failed to remove local test data dir %q: %s", env, err)
		}
	})

	return env, iEnv, node, core, unwind
}
