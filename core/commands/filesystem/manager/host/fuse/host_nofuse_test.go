//+build nofuse

package fuse_test

import (
	"testing"

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/fuse"
)

func testProvider(t *testing.T) {
	if _, err := fuse.HostMounter(nil, nil); err != fuse.ErrNoFuse {
		t.Fatalf("nofuse tag enabled but provider did not return appropriate error: %#v", err)
	}
}
