package providercommon_test

import (
	"testing"

	mountinter "github.com/ipfs/go-ipfs/filesystem/mount"
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
	// acquire lock
	if err := locker.Request(namespace, target, lType, timeout); err != nil {
		t.Error(err)
		t.FailNow()
	}

	// should fail to acquire lock
	if err := locker.Request(namespace, target, lType, timeout); err == nil {
		t.Error("acquired lock when already acquired")
		t.FailNow()
	} else {
		t.Logf("Intentionally failed to acquire lock: %s", err)
	}

	// should not panic
	locker.Release(namespace, target, lType)

	// acquire lock again
	if err := locker.Request(namespace, target, lType, timeout); err != nil {
		t.Error(err)
		t.FailNow()
	}

	// should not panic
	locker.Release(namespace, target, lType)
}
