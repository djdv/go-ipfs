package filesystem

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	gopath "path"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/djdv/p9/p9"
	files "github.com/ipfs/go-ipfs-files"

	"github.com/ipfs/go-ipfs/core"
	"github.com/ipfs/go-ipfs/core/coreapi"
	fsnodes "github.com/ipfs/go-ipfs/plugin/plugins/filesystem/nodes"
	logging "github.com/ipfs/go-log"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	coreoptions "github.com/ipfs/interface-go-ipfs-core/options"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

func TestAll(t *testing.T) {
	ctx := context.TODO()
	core, err := initCore(ctx)
	if err != nil {
		t.Fatalf("Failed to construct IPFS node: %s\n", err)
	}

	logger := logging.Logger("plugin/filesystem")

	t.Run("RootFS", func(t *testing.T) { testRootFS(t, ctx, core, logger) })
	t.Run("PinFS", func(t *testing.T) { testPinFS(t, ctx, core, logger) })
}

func testRootFS(t *testing.T, ctx context.Context, core coreiface.CoreAPI, logger logging.EventLogger) {
	ri, err := fsnodes.NewRoot(ctx, core, logger)
	if err != nil {
		t.Fatalf("Failed to attach to 9P root resource: %s\n", err)
	}

	nineRoot, err := ri.Attach()
	_, nineRef, err := nineRoot.Walk(nil)
	if err != nil {
		t.Fatalf("Failed to walk root: %s\n", err)
	}
	if _, _, err = nineRef.Open(p9.ReadOnly); err != nil {
		t.Fatalf("Failed to open root: %s\n", err)
	}

	ents, err := nineRef.Readdir(0, ^uint32(0))
	if err != nil {
		t.Fatalf("Failed to read root: %s\n", err)
	}

	//TODO: currently magic, as subsystems are implemented, rework this part of the test + lib
	if len(ents) != 1 || ents[0].Name != "ipfs" {
		t.Fatalf("Failed, root has bad entries:: %v\n", ents)
	}

	//TODO: type checking
}

func testPinFS(t *testing.T, ctx context.Context, core coreiface.CoreAPI, logger logging.EventLogger) {
	//init
	pinRoot, err := fsnodes.InitPinFS(ctx, core, logger).Attach()
	if err != nil {
		t.Fatalf("Failed to attach to 9P Pin resource: %s\n", err)
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
			t.Fatalf("Failed to list IPFS pins: %s\n", err)
		}
		p9Pins, err := p9PinNames(pinRoot)
		if err != nil {
			t.Fatalf("Failed to list 9P pins: %s\n", err)
		}

		if !same(basePins, p9Pins) {
			t.Fatalf("Pinsets differ\ncore: %v\n9P: %v\n", basePins, p9Pins)
		}
	}

	//test default (likely empty) test repo pins
	shallowCompare()

	// test modifying pinset +1; initEnv pins its IPFS envrionment
	env, _, err := initEnv(ctx, core)
	if err != nil {
		t.Fatalf("Failed to construct IPFS test environment: %s\n", err)
	}
	defer os.RemoveAll(env)
	shallowCompare()

	// test modifying pinset +1 again; generate garbage and pin it
	{
		if err := generateGarbage(env); err != nil {
			t.Fatalf("Failed to generate test data: %s\n", err)
		}

		_, err := pinAddDir(ctx, core, env)
		if err != nil {
			t.Fatalf("Failed to add directory to IPFS: %s\n", err)
		}
	}
	shallowCompare()

	//TODO: type checking
}
func TestIPFS(t *testing.T) {
	ctx := context.TODO()
	core, err := initCore(ctx)
	if err != nil {
		t.Fatalf("Failed to construct IPFS node: %s\n", err)
	}

	env, iEnv, err := initEnv(ctx, core)
	if err != nil {
		t.Fatalf("Failed to construct IPFS test environment: %s\n", err)
	}

	t.Logf("env:%v\niEnv:%v\nerr:%s\n", env, iEnv, err)
	defer os.RemoveAll(env)
}

func initCore(ctx context.Context) (coreiface.CoreAPI, error) {
	node, err := core.NewNode(ctx, &core.BuildCfg{
		Online:                      false,
		Permanent:                   false,
		DisableEncryptedConnections: true,
	})
	if err != nil {
		return nil, err
	}

	return coreapi.NewCoreAPI(node)
}

const incantation = "May the bits passing through this device somehow help bring peace to this world"

func initEnv(ctx context.Context, core coreiface.CoreAPI) (string, corepath.Resolved, error) {
	tempDir, err := ioutil.TempDir("", "ipfs-")
	if err != nil {
		return "", nil, err
	}

	if err = ioutil.WriteFile(filepath.Join(tempDir, "empty"),
		[]byte(nil),
		0644); err != nil {
		return "", nil, err
	}

	if err = ioutil.WriteFile(filepath.Join(tempDir, "small"),
		[]byte(incantation),
		0644); err != nil {
		return "", nil, err
	}

	if err := generateGarbage(tempDir); err != nil {
		return "", nil, err
	}

	iPath, err := pinAddDir(ctx, core, tempDir)
	if err != nil {
		return "", nil, err
	}

	return tempDir, iPath, err
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

func pinNames(ctx context.Context, core coreiface.CoreAPI) ([]string, error) {
	pins, err := core.Pin().Ls(ctx, coreoptions.Pin.Type.Recursive())
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(pins))
	for _, pin := range pins {
		names = append(names, gopath.Base(pin.Path().String()))
	}
	return names, nil
}

func p9PinNames(root p9.File) ([]string, error) {
	_, rootDir, err := root.Walk(nil)
	if err != nil {
		return nil, err
	}

	_, _, err = rootDir.Open(p9.ReadOnly)
	if err != nil {
		return nil, err
	}
	ents, err := rootDir.Readdir(0, ^uint32(0))
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(ents))

	for _, ent := range ents {
		names = append(names, ent.Name)
	}

	return names, rootDir.Close()
}
