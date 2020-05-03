package ipfscore

import (
	"context"
	"os"
	gopath "path"
	"reflect"
	"sort"
	"testing"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	testutil "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems/internal/testutils"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

func TestAll(t *testing.T) {
	env, iEnv, node, core, unwind := testutil.GenerateTestEnv(t)
	defer node.Close()
	t.Cleanup(unwind)

	ctx := context.TODO()

	t.Run("IPFS", func(t *testing.T) { testIPFS(ctx, t, env, iEnv, core) })
}

func testIPFS(ctx context.Context, t *testing.T, env string, iEnv corepath.Resolved, core coreiface.CoreAPI) {

	initChan := make(fusecom.InitSignal)
	fuseIPFS := NewFileSystem(ctx, core,
		WithNamespace(mountinter.NamespaceIPFS),
		WithCommon(
			fusecom.WithInitSignal(initChan),
			// fusecom.WithResourceLock(fs.IPFSCore), TODO
		),
	)

	go fuseIPFS.Init()
	for err := range initChan {
		if err != nil {
			t.Logf("subsystem init failed:%s\n", err)
			t.FailNow()
		}
	}

	iDir := gopath.Base(iEnv.String())

	localDir, err := os.Open(env)
	if err != nil {
		t.Logf("failed to open local environment: %s\n", err)
		t.FailNow()
	}

	localEntries, err := localDir.Readdirnames(0)
	if err != nil {
		t.Logf("failed to read local environment: %s\n", err)
		t.FailNow()
	}
	sort.Strings(localEntries)

	errNo, dirHandle := fuseIPFS.Opendir(iDir)
	if errNo != fusecom.OperationSuccess {
		t.Logf("Opendir failed (status: %s) opening IPFS hash %q\n", fuselib.Error(errNo), iDir)
		t.FailNow()
	}

	genFill := func(slice *[]string) func(name string, stat *fuselib.Stat_t, ofst int64) bool {
		return func(name string, _ *fuselib.Stat_t, _ int64) bool {
			*slice = append(*slice, name)
			return true
		}
	}

	coreEntries := make([]string, 0, len(localEntries))
	filler := genFill(&coreEntries)

	// read everything
	var offsetVal int64 = 0
	if errNo = fuseIPFS.Readdir(iDir, filler, offsetVal, dirHandle); errNo != fusecom.OperationSuccess {
		t.Logf("Readdir failed (status: %s) reading {%#x|%q} with offset %d\n", fuselib.Error(errNo), dirHandle, iDir, offsetVal)
		t.FailNow()
	}

	// compare core list with local list
	// (Go doesn't include dots, and order is not gauranteed so account for this)
	sortedCoreEntries := make([]string, len(coreEntries))
	copy(sortedCoreEntries, coreEntries)
	sort.Strings(sortedCoreEntries)
	sort.Strings(localEntries)
	sortedCoreEntries = sortedCoreEntries[2:] // trim off dots

	if !reflect.DeepEqual(sortedCoreEntries, localEntries) {
		t.Logf("entries within directory do not match\nexpected:%v\nhave:%v", localEntries, sortedCoreEntries)
		t.FailNow()
	}

	/* FIXME: this is all messed up, skipping dots passes an offset of 0 to the underlying stream reader
	the dot Filler is going to need to manage cache state directly

	offsetVal = 2 // skip dots
	partialList := make([]string, 0, len(entryList)-offsetVal)
	filler = genFill(&partialList)
	*/

	// TODO range this, read all offsets forward, backwards, then forward again
	{
		offsetVal = 3 // skip dots, and first IPFS entry
		partialList := make([]string, 0, int64(len(coreEntries))-offsetVal)
		filler = genFill(&partialList)

		// read back the same directory using an offset, contents should match
		if errNo = fuseIPFS.Readdir(iDir, filler, offsetVal, dirHandle); errNo != fusecom.OperationSuccess {
			t.Logf("Readdir failed (status: %s) reading {%#x|%q} with offset %d\n", fuselib.Error(errNo), dirHandle, iDir, offsetVal)
			t.FailNow()
		}

		// providing an offset should replay the stream exactly; no sorting should occur
		coreSubSlice := coreEntries[offsetVal:]
		if !reflect.DeepEqual(coreSubSlice, partialList) {
			t.Logf("offset entries does not match\nexpected:%v\nhave:%v", coreSubSlice, partialList)
			t.FailNow()
		}

		offsetVal = int64(len(coreEntries)) - 1 // last entry only
		partialList = make([]string, 0, int64(len(coreEntries))-offsetVal)
		filler = genFill(&partialList)

		// read back the same directory using an offset, contents should match
		if errNo = fuseIPFS.Readdir(iDir, filler, offsetVal, dirHandle); errNo != fusecom.OperationSuccess {
			t.Logf("Readdir failed (status: %s) reading {%#x|%q} with offset %d\n", fuselib.Error(errNo), dirHandle, iDir, offsetVal)
			t.FailNow()
		}

		coreSubSlice = coreEntries[offsetVal:]
		if !reflect.DeepEqual(coreSubSlice, partialList) {
			t.Logf("offset entries does not match\nexpected:%v\nhave:%v", coreSubSlice, partialList)
			t.FailNow()
		}

		offsetVal = 3 // skip dots, and first IPFS entry
		partialList = make([]string, 0, int64(len(coreEntries))-offsetVal)
		filler = genFill(&partialList)

		// read back the same directory using an offset, contents should match
		if errNo = fuseIPFS.Readdir(iDir, filler, offsetVal, dirHandle); errNo != fusecom.OperationSuccess {
			t.Logf("Readdir failed (status: %s) reading {%#x|%q} with offset %d\n", fuselib.Error(errNo), dirHandle, iDir, offsetVal)
			t.FailNow()
		}

		coreSubSlice = coreEntries[offsetVal:]
		if !reflect.DeepEqual(coreSubSlice, partialList) {
			t.Logf("offset entries does not match\nexpected:%v\nhave:%v", coreSubSlice, partialList)
			t.FailNow()
		}

	}

}
