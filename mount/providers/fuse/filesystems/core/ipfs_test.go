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

func testIPFS(ctx context.Context, t *testing.T, env string, coreEnv corepath.Resolved, core coreiface.CoreAPI) {

	initChan := make(fusecom.InitSignal)
	fs := NewFileSystem(ctx, core,
		WithNamespace(mountinter.NamespaceIPFS),
		WithCommon(
			fusecom.WithInitSignal(initChan),
			// fusecom.WithResourceLock(fs.IPFSCore), TODO
		),
	)

	go fs.Init()
	for err := range initChan {
		if err != nil {
			t.Fatalf("subsystem init failed:%s\n", err)
		}
	}

	corePath := gopath.Base(coreEnv.String())
	t.Run("Readdir", func(t *testing.T) { testReaddir(t, env, corePath, fs) })
}

type fileHandle = uint64
type readdirTestDirEnt struct {
	name   string
	offset int64
}

func genFill(slice *[]readdirTestDirEnt) func(name string, stat *fuselib.Stat_t, ofst int64) bool {
	return func(name string, _ *fuselib.Stat_t, ofst int64) bool {
		// buffer is full
		if cap(*slice) == 0 {
			return false
		}
		if len(*slice) == cap(*slice) {
			return false
		}

		// populate
		*slice = append(*slice, readdirTestDirEnt{name, ofst})

		// buffer still has free space?
		return len(*slice) != cap(*slice)
	}
}

func testReaddir(t *testing.T, localPath, corePath string, fs *FileSystem) {
	// setup
	localDir, err := os.Open(localPath)
	if err != nil {
		t.Fatalf("failed to open local environment: %s\n", err)
	}

	localEntries, err := localDir.Readdirnames(0)
	if err != nil {
		t.Fatalf("failed to read local environment: %s\n", err)
	}
	sort.Strings(localEntries)

	{ // instance 1
		errNo, dirHandle := fs.Opendir(corePath)
		if errNo != fusecom.OperationSuccess {
			t.Fatalf("Opendir failed (status: %s) opening %q\n", fuselib.Error(errNo), corePath)
		}

		// make sure we can read the directory completley, in one call
		var coreEntries []readdirTestDirEnt
		t.Run("all at once", func(t *testing.T) {
			coreEntries = testReaddirAll(t, localEntries, fs, corePath, dirHandle)
		})

		// check that reading with an offset replays the stream exactly
		t.Run("with offset", func(t *testing.T) {
			testReaddirOffset(t, coreEntries, fs, corePath, dirHandle)
		})

		// we're done with this instance
		if errNo := fs.Releasedir(corePath, dirHandle); errNo != fusecom.OperationSuccess {
			t.Fatalf("Releasedir failed (status: %s) closing %q\n", fuselib.Error(errNo), corePath)
		}
	}

	{ // instance 2
		errNo, dirHandle := fs.Opendir(corePath)
		if errNo != fusecom.OperationSuccess {
			t.Fatalf("Opendir failed (status: %s) opening %q\n", fuselib.Error(errNo), corePath)
		}

		// test reading 1 by 1
		t.Run("incremental", func(t *testing.T) {
			testReaddirAllIncremental(t, localEntries, fs, corePath, dirHandle)
		})

		// we only need this for comparison
		coreEntries := testReaddirAll(t, localEntries, fs, corePath, dirHandle)

		// check that reading incrementally with an offset replays the stream exactly
		t.Run("incrementally with offset", func(t *testing.T) {
			testReaddirIncrementalOffset(t, coreEntries, fs, corePath, dirHandle)
		})

		// we're done with this instance
		if errNo := fs.Releasedir(corePath, dirHandle); errNo != fusecom.OperationSuccess {
			t.Fatalf("Releasedir failed (status: %s) closing %q\n", fuselib.Error(errNo), corePath)
		}
	}
}

func testReaddirAll(t *testing.T, expected []string, fs *FileSystem, corePath string, fh fileHandle) []readdirTestDirEnt {
	coreEntries := make([]readdirTestDirEnt, 0, len(expected)+2) // + '.', ".."
	filler := genFill(&coreEntries)

	const offsetVal = 0
	if errNo := fs.Readdir(corePath, filler, offsetVal, fh); errNo != fusecom.OperationSuccess {
		t.Fatalf("Readdir failed (status: %s) reading {%#x|%q} with offset %d\n", fuselib.Error(errNo), fh, corePath, offsetVal)
	}

	// entries are not expected to be sorted from either source
	// we'll make and munge copies so we don't alter the source inputs
	sortedExpectationsAndDreams := make([]string, len(expected))
	copy(sortedExpectationsAndDreams, expected)

	sortedCoreEntries := make([]string, 0, len(expected))
	for _, ent := range coreEntries {
		// (Go's `Readnames` doesn't include dots, so exclude them)
		if ent.name == "." || ent.name == ".." {
			continue
		}
		sortedCoreEntries = append(sortedCoreEntries, ent.name)
	}

	// in-place sort actual
	sort.Strings(sortedCoreEntries)
	sort.Strings(sortedExpectationsAndDreams)

	// actual comparison
	if !reflect.DeepEqual(sortedExpectationsAndDreams, sortedCoreEntries) {
		t.Fatalf("entries within directory do not match\nexpected:%v\nhave:%v", sortedExpectationsAndDreams, sortedCoreEntries)
	}

	t.Logf("%v\n", coreEntries)
	return coreEntries
}

func testReaddirOffset(t *testing.T, existing []readdirTestDirEnt, fs *FileSystem, corePath string, fh fileHandle) {
	partialList := make([]readdirTestDirEnt, 0, len(existing)-1)
	filler := genFill(&partialList)

	offsetVal := existing[0].offset
	// read back the same entries. starting at an offset, contents should match
	if errNo := fs.Readdir(corePath, filler, offsetVal, fh); errNo != fusecom.OperationSuccess {
		t.Fatalf("Readdir failed (status: %s) reading {%#x|%q} with offset %d\n", fuselib.Error(errNo), fh, corePath, offsetVal)
	}

	// providing an offset should replay the stream exactly; no sorting should occur
	if !reflect.DeepEqual(existing[1:], partialList) {
		t.Fatalf("offset entries do not match\nexpected:%v\nhave:%v", existing[1:], partialList)
	}

	t.Logf("%v\n", partialList)
}

func genShortFill(slice *[]readdirTestDirEnt) func(name string, stat *fuselib.Stat_t, ofst int64) bool {
	return func(name string, _ *fuselib.Stat_t, ofst int64) bool {
		*slice = append(*slice, readdirTestDirEnt{name, ofst})
		return false // buffer is full
	}
}

func testReaddirAllIncremental(t *testing.T, expected []string, fs *FileSystem, corePath string, fh fileHandle) {
	var (
		offsetVal  int64
		entNames   = make([]string, 0, len(expected))
		loggedEnts = make([]readdirTestDirEnt, 0, len(expected)+2) // + '.', ".."
	)

	for {
		singleEnt := make([]readdirTestDirEnt, 0, 1)
		filler := genShortFill(&singleEnt)

		if errNo := fs.Readdir(corePath, filler, offsetVal, fh); errNo != fusecom.OperationSuccess {
			t.Fatalf("Readdir failed (status: %s) reading {%#x|%q} with offset %d\n", fuselib.Error(errNo), fh, corePath, offsetVal)
		}

		if len(singleEnt) == 0 {
			// Readdir didn't fail but filled in nothing; (equivalent of `readdir() == NULL`)
			break
		}

		if len(singleEnt) != 1 {
			t.Fatalf("Readdir did not respect fill() stop signal (buffer overflowed)")
		}

		entNames = append(entNames, singleEnt[0].name)
		loggedEnts = append(loggedEnts, singleEnt...)
		offsetVal = singleEnt[0].offset
	}

	// entries are not expected to be sorted from either source
	// we'll make and munge copies so we don't alter the source inputs
	sortedExpectationsAndDreams := make([]string, len(expected))
	copy(sortedExpectationsAndDreams, expected)

	// in-place sort actual
	sort.Strings(entNames)
	sort.Strings(sortedExpectationsAndDreams)

	// remove dots from core names
	entNames = entNames[2:]

	// actual comparison
	if !reflect.DeepEqual(sortedExpectationsAndDreams, entNames) {
		t.Fatalf("entries within directory do not match\nexpected:%v\nhave:%v", sortedExpectationsAndDreams, entNames)
	}
	t.Logf("%v\n", loggedEnts)
}

func testReaddirIncrementalOffset(t *testing.T, existing []readdirTestDirEnt, fs *FileSystem, corePath string, fh fileHandle) {
	compareBuffer := make([]readdirTestDirEnt, 0, int64(len(existing)-1))

	for _, ent := range existing {
		offsetVal := ent.offset
		singleEnt := make([]readdirTestDirEnt, 0, 1)
		shortFiller := genShortFill(&singleEnt)

		if errNo := fs.Readdir(corePath, shortFiller, offsetVal, fh); errNo != fusecom.OperationSuccess {
			t.Fatalf("Readdir failed (status: %s) reading {%#x|%q} with offset %d\n", fuselib.Error(errNo), fh, corePath, offsetVal)
		}

		if len(singleEnt) == 0 {
			// Readdir didn't fail but filled in nothing; (equivalent of `readdir() == NULL`)
			break
		}

		if len(singleEnt) != 1 {
			t.Fatalf("Readdir did not respect fill() stop signal (buffer overflowed)")
		}

		compareBuffer = append(compareBuffer, singleEnt[0])
	}

	if !reflect.DeepEqual(existing[1:], compareBuffer) {
		t.Fatalf("offset entries do not match\nexpected:%v\nhave:%v", existing[1:], compareBuffer)
	}

	t.Logf("%v\n", compareBuffer)
}
