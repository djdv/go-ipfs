package mountfuse

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	fuselib "github.com/billziss-gh/cgofuse/fuse"
	files "github.com/ipfs/go-ipfs-files"
	fusecom "github.com/ipfs/go-ipfs/mount/providers/fuse/filesystems"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	coreoptions "github.com/ipfs/interface-go-ipfs-core/options"
)

func hashLocal(t *testing.T, localFilePath string, core coreiface.CoreAPI) string {
	// XXX: magic path string and reconstruction of existing data
	// these files should be part of some associative array of structs returned by the test lib
	// testFileArray[const testlib.small].(files.File)CoreFile;  [].(string)LocalPath; [].(corepath.Path)CorePath

	fi, err := os.Stat(localFilePath)
	if err != nil {
		t.Errorf("failed to stat local file %q: %s\n", localFilePath, err)
	}

	fileNode, err := files.NewSerialFile(localFilePath, false, fi)
	if err != nil {
		t.Errorf("failed to wrap local file %q: %s\n", localFilePath, err)
	}

	ipfsFilePath, err := core.Unixfs().Add(context.TODO(), fileNode.(files.File), coreoptions.Unixfs.HashOnly(true))
	if err != nil {
		t.Errorf("failed to hash local file %q: %s\n", localFilePath, err)
	}

	return ipfsFilePath.Cid().String()
}

func testFiles(t *testing.T, localPath string, core coreiface.CoreAPI, fs fuselib.FileSystemInterface) {
	localFilePath := filepath.Join(localPath, "small")
	fileHash := hashLocal(t, localFilePath, core)

	t.Run("Open+Release", func(t *testing.T) {
		// TODO: test a bunch of scenarios/flags as separate runs here
		// t.Run("with O_CREAT"), "Write flags", etc...
		fh := testOpen(t, fileHash, fuselib.O_RDONLY, fs)
		testRelease(t, fileHash, fh, fs)
	})

	mirror, err := os.Open(localFilePath)
	if err != nil {
		t.Errorf("failed to open local file %q: %s\n", localFilePath, err)
	}

	t.Run("Read", func(t *testing.T) {
		fh := testOpen(t, fileHash, fuselib.O_RDONLY, fs)
		testRead(t, fileHash, mirror, fh, fs)
	})
	if err := mirror.Close(); err != nil {
		t.Errorf("failed to close local file %q: %s\n", localFilePath, err)
	}
}

func testOpen(t *testing.T, path string, flags int, fs fuselib.FileSystemInterface) fileHandle {
	errno, fh := fs.Open(path, flags)
	if errno != fusecom.OperationSuccess {
		t.Errorf("failed to open %q: %s\n", path, fuselib.Error(errno))
	}
	return fh
}

func testRelease(t *testing.T, path string, fh fileHandle, fs fuselib.FileSystemInterface) int {
	errno := fs.Release(path, fh)
	if errno != fusecom.OperationSuccess {
		t.Errorf("failed to close %q: %s\n", path, fuselib.Error(errno))
	}
	return errno
}

func testRead(t *testing.T, path string, mirror *os.File, fh fileHandle, fs fuselib.FileSystemInterface) {
	t.Run("all", func(t *testing.T) {
		testReadAll(t, path, mirror, fh, fs)
	})

	mirror.Seek(0, 0)
}

func testReadAll(t *testing.T, path string, mirror *os.File, fh fileHandle, fs fuselib.FileSystemInterface) {
	expected, err := ioutil.ReadAll(mirror)
	if err != nil {
		t.Errorf("failed to read mirror contents: %s\n", err)
	}

	fullBuff := make([]byte, len(expected))

	readRet := fs.Read(path, fullBuff, 0, fh)
	if readRet < 0 {
		t.Errorf("failed to read %q: %s\n", path, fuselib.Error(readRet))
	}

	// FIXME: [temporary] don't assume full reads in one shot; this isn't spec compliant
	// we need to loop until EOF
	if readRet != len(expected) || readRet != len(fullBuff) {
		t.Errorf("read bytes does not match actual length of bytes buffer for %q:\nexpected:%d\nhave:%d\n", path, len(expected), readRet)
	}

	// TODO: make sure to change this error message when we start testing large files
	if !reflect.DeepEqual(expected, fullBuff) {
		t.Errorf("contents for %q do not match:\nexpected:%v\nhave:%v\n", path, expected, fullBuff)
	}

	t.Logf("%s\n", fullBuff)
}
