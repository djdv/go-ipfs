package mountcom

import (
	"testing"

	mountinter "github.com/ipfs/go-ipfs/mount/interface"
)

func TestAll(t *testing.T) {
	locker := NewResourceLocker()
	t.Run("Acquire", func(t *testing.T) { testAcquire(t, locker) })
}

func testAcquire(t *testing.T, locker ResourceLock) {
	const (
		namespace = mountinter.NamespaceIPFS
		target    = "/lock/test/path"
		lType     = LockDataWrite
		timeout   = 0
	)
	// aquire lock
	if err := locker.Request(namespace, target, lType, timeout); err != nil {
		t.Error(err)
		t.FailNow()
	}

	// should fail to aquire lock
	if err := locker.Request(namespace, target, lType, timeout); err == nil {
		t.Error("acquired lock when already acquired")
		t.FailNow()
	} else {
		t.Logf("Intentionally failed to aquire lock: %s", err)
	}

	// should not panic
	locker.Release(namespace, target, lType)

	// aquire lock again
	if err := locker.Request(namespace, target, lType, timeout); err != nil {
		t.Error(err)
		t.FailNow()
	}

	// should not panic
	locker.Release(namespace, target, lType)
}
