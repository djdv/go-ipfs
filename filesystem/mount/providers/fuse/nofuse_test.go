//+build nofuse

package fuse_test

import (
	"testing"
)

func TestAll(t *testing.T) {
	t.Run("interface provider stub", testProvider)
}
