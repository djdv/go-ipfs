//+build nofuse

package fuse_test

import (
	"testing"

	mountinter "github.com/ipfs/go-ipfs/core/commands/filesystem"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/fuse"
)

func testProvider(t *testing.T) {
	if _, err := fuse.NewProvider(nil, mountinter.NamespaceIPFS, "", nil); err != fuse.ErrNoFuse {
		t.Fatalf("nofuse tag enabled but provider did not return appropriate error: %#v", err)
	}
}
