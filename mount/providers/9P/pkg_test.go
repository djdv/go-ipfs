package mount9p

/* not ported over yet
import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	gopath "path"
	"path/filepath"
	"testing"
	"time"

	"github.com/hugelgupf/p9/p9"
	files "github.com/ipfs/go-ipfs-files"
	nodeopts "github.com/ipfs/go-ipfs/mount/providers/9P/common"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	coreoptions "github.com/ipfs/interface-go-ipfs-core/options"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

const (
	errFmtRoot       = "Failed to attach to 9P root resource: %s\n"
	errFmtRootSecond = "Failed to attach to 9P root resource a second time: %s\n"
	errFmtClose      = "Close errored: %s\n"
	errFmtClone      = "Failed to clone ref: %s\n"
)

// TODO: [port from plugin] salvage fs tests; discard plugin tests

/* TODO:
add a set of standard tests that have redundant functionlity for transforms
e.g.
StandardQIDFromStringFunction("/pinfs") == pinfsObject.QIDMethod()
*/
/*
type baseAttacher func(context.Context, coreiface.CoreAPI, ...nodeopts.AttachOption) p9.Attacher

func baseline(ctx context.Context, t *testing.T, core coreiface.CoreAPI, attachFn baseAttacher) {
	attacher := attachFn(ctx, core)

	t.Run("attacher", func(t *testing.T) { testAttacher(ctx, t, attacher) })

	root, err := attacher.Attach()
	if err != nil {
		t.Logf("Attach test passed but attach failed: %s\n", err)
		t.FailNow()
	}

	t.Run("walk", func(t *testing.T) { testClones(ctx, t, root) })
	t.Run("open", func(t *testing.T) { testOpen(ctx, t, root) })

	if _, _, _, err = root.GetAttr(p9.AttrMaskAll); err != nil {
		t.Log(err)
		t.FailNow()
	}
}

func testAttacher(ctx context.Context, t *testing.T, attacher p9.Attacher) {
	// 2 individual instances, one after another
	nineRoot, err := attacher.Attach()
	if err != nil {
		t.Logf(errFmtRoot, err)
		t.FailNow()
	}

	if err = nineRoot.Close(); err != nil {
		t.Logf(errFmtClose, err)
		t.FailNow()
	}

	nineRootTheRevenge, err := attacher.Attach()
	if err != nil {
		t.Logf(errFmtRootSecond, err)
		t.FailNow()
	}

	if err = nineRootTheRevenge.Close(); err != nil {
		t.Logf(errFmtClose, err)
		t.FailNow()
	}

	// 2 instances at the same time
	nineRoot, err = attacher.Attach()
	if err != nil {
		t.Logf(errFmtRoot, err)
		t.FailNow()
	}

	nineRootTheRevenge, err = attacher.Attach()
	if err != nil {
		t.Logf(errFmtRootSecond, err)
		t.FailNow()
	}

	if err = nineRootTheRevenge.Close(); err != nil {
		t.Logf(errFmtClose, err)
		t.FailNow()
	}

	if err = nineRoot.Close(); err != nil {
		t.Logf(errFmtClose, err)
		t.FailNow()
	}

	// final instance
	nineRoot, err = attacher.Attach()
	if err != nil {
		t.Logf(errFmtRoot, err)
		t.FailNow()
	}

	if err = nineRoot.Close(); err != nil {
		t.Logf(errFmtClose, err)
		t.FailNow()
	}
}

func testClones(ctx context.Context, t *testing.T, nineRef p9.File) {

	// clone the node we were passed; 1st generation
	_, newRef, err := nineRef.Walk(nil)
	if err != nil {
		t.Logf(errFmtClone, err)
		t.FailNow()
	}

	// this `Close` shouldn't affect the parent it's derived from
	// only descendants
	if err = newRef.Close(); err != nil {
		t.Logf(errFmtClose, err)
		t.FailNow()
	}

	// remake the clone from the original; 1st generation again
	_, gen1, err := nineRef.Walk(nil)
	if err != nil {
		t.Logf(errFmtClone, err)
		t.FailNow()
	}

	// clone a 2nd generation from the 1st
	_, gen2, err := gen1.Walk(nil)
	if err != nil {
		t.Logf(errFmtClone, err)
		t.FailNow()
	}

	// 3rd from the 2nd
	_, gen3, err := gen2.Walk(nil)
	if err != nil {
		t.Logf(errFmtClone, err)
		t.FailNow()
	}

	// close the 2nd reference
	if err = gen2.Close(); err != nil {
		t.Logf(errFmtClose, err)
		t.FailNow()
	}

	// try to clone from the 2nd reference
	// this should fail since we closed it
	_, undead, err := gen2.Walk(nil)
	if err == nil {
		t.Logf("Clone (%p)%q succeeded when parent (%p)%q was closed\n", undead, undead, gen2, gen2)
		t.FailNow()
	}

	// 4th from  the 3rd
	// should still succeed regardless of 2's state
	_, gen4, err := gen3.Walk(nil)
	if err != nil {
		t.Logf(errFmtClone, err)
		t.FailNow()
	}

	// close the 3rd reference
	if err = gen3.Close(); err != nil {
		t.Logf(errFmtClose, err)
		t.FailNow()
	}

	// close the 4th reference
	if err = gen4.Close(); err != nil {
		t.Logf(errFmtClose, err)
		t.FailNow()
	}

	// clone a 2nd generation from the 1st again
	_, gen2, err = gen1.Walk(nil)
	if err != nil {
		t.Logf(errFmtClone, err)
		t.FailNow()
	}

	// close the 1st
	if err = gen1.Close(); err != nil {
		t.Logf(errFmtClose, err)
		t.FailNow()
	}

	// close the 2nd
	if err = gen2.Close(); err != nil {
		t.Logf(errFmtClose, err)
		t.FailNow()
	}
}

func testOpen(ctx context.Context, t *testing.T, nineRef p9.File) {
	_, newRef, err := nineRef.Walk(nil)
	if err != nil {
		t.Logf(errFmtClone, err)
		t.FailNow()
	}

	_, thing1, err := nineRef.Walk(nil)
	if err != nil {
		t.Logf(errFmtClone, err)
		t.FailNow()
	}
	_, thing2, err := nineRef.Walk(nil)
	if err != nil {
		t.Logf(errFmtClone, err)
		t.FailNow()
	}

	// a close of one reference should not affect the operation context of another
	if err = thing1.Close(); err != nil {
		t.Logf(errFmtClose, err)
		t.FailNow()
	}

	if _, _, err = thing2.Open(0); err != nil {
		t.Logf("could not open reference after unrelated reference was closed: %s\n", err)
		t.FailNow()
	}

	// cleanup
	if err = thing2.Close(); err != nil {
		t.Logf(errFmtClose, err)
		t.FailNow()
	}

	if err = newRef.Close(); err != nil {
		t.Logf(errFmtClose, err)
		t.FailNow()
	}
}

func testCompareTreeAttrs(t *testing.T, f1, f2 p9.File) {
	var expand func(p9.File) (map[string]p9.Attr, error)
	expand = func(nineRef p9.File) (map[string]p9.Attr, error) {
		ents, err := p9Readdir(nineRef)
		if err != nil {
			return nil, err
		}

		res := make(map[string]p9.Attr)
		for _, ent := range ents {
			_, child, err := nineRef.Walk([]string{ent.Name})
			if err != nil {
				return nil, err
			}

			_, _, attr, err := child.GetAttr(attrMaskIPFSTest)
			if err != nil {
				return nil, err
			}
			res[ent.Name] = attr

			if ent.Type == p9.TypeDir {
				subRes, err := expand(child)
				if err != nil {
					return nil, err
				}
				for name, attr := range subRes {
					res[gopath.Join(ent.Name, name)] = attr
				}
			}
			if err = child.Close(); err != nil {
				return nil, err
			}
		}
		return res, nil
	}

	f1Map, err := expand(f1)
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	f2Map, err := expand(f2)
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	same := func(permissionContains p9.FileMode, base, target map[string]p9.Attr) bool {
		if len(base) != len(target) {

			var baseNames []string
			var targetNames []string
			for name := range base {
				baseNames = append(baseNames, name)
			}
			for name := range target {
				targetNames = append(targetNames, name)
			}

			t.Logf("map lengths don't match:\nbase:%v\ntarget:%v\n", baseNames, targetNames)
			t.FailNow()
			return false
		}

		for path, baseAttr := range base {
			bMode := baseAttr.Mode
			tMode := target[path].Mode

			if bMode.FileType() != tMode.FileType() {
				t.Logf("type for %q don't match:\nbase:%v\ntarget:%v\n", path, bMode, tMode)
				t.FailNow()
				return false
			}

			if ((bMode.Permissions() & permissionContains) & (tMode.Permissions() & permissionContains)) == 0 {
				t.Logf("permissions for %q don't match\n(unfiltered)\nbase:%v\ntarget:%v\n(filtered)\nbase:%v\ntarget:%v\n",
					path,
					bMode.Permissions(), tMode.Permissions(),
					bMode.Permissions()&permissionContains, tMode.Permissions()&permissionContains,
				)
				t.FailNow()
				return false
			}

			if bMode.FileType() != p9.ModeDirectory {
				bSize := baseAttr.Size
				tSize := target[path].Size

				if bSize != tSize {
					t.Logf("size for %q doesn't match\nbase:%d\ntarget:%d\n",
						path,
						bSize,
						tSize)
					t.FailNow()
				}
			}
		}
		return true
	}
	if !same(p9.Read, f1Map, f2Map) {
		t.Logf("contents don't match \nf1:%v\nf2:%v\n", f1Map, f2Map)
		t.FailNow()
	}
}

const incantation = "May the bits passing through this device somehow help bring peace to this world"

func initEnv(ctx context.Context, core coreiface.CoreAPI) (string, corepath.Resolved, error) {
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

func p9Readdir(dir p9.File) (p9.Dirents, error) {
	_, dirClone, err := dir.Walk(nil)
	if err != nil {
		return nil, err
	}

	_, _, err = dirClone.Open(p9.ReadOnly)
	if err != nil {
		return nil, err
	}
	defer dirClone.Close()

	var (
		offset uint64
		ents   p9.Dirents
	)
	for {
		var curEnts p9.Dirents
		curEnts, err = dirClone.Readdir(offset, ^uint32(0))
		lEnts := len(curEnts)
		if err != nil || lEnts == 0 {
			break
		}

		ents = append(ents, curEnts...)
		offset += uint64(lEnts)
	}

	return ents, err
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
	ents, err := p9Readdir(root)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(ents))

	for _, ent := range ents {
		names = append(names, ent.Name)
	}

	return names, nil
}

//TODO:
// NOTE: compares a subset of attributes, matching those of IPFS
func testIPFSCompare(t *testing.T, f1, f2 p9.File) {
	_, _, f1Attr, err := f1.GetAttr(attrMaskIPFSTest)
	if err != nil {
		t.Errorf("Attr(%v) = %v, want nil", f1, err)
	}
	_, _, f2Attr, err := f2.GetAttr(attrMaskIPFSTest)
	if err != nil {
		t.Errorf("Attr(%v) = %v, want nil", f2, err)
	}
	if f1Attr != f2Attr {
		t.Errorf("Attributes of same files do not match: %v and %v", f1Attr, f2Attr)
	}
}
*/