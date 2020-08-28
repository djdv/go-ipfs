package mountcom_test

import (
	"testing"

	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	mountcom "github.com/ipfs/go-ipfs/mount/utils/common"
)

func TestAll(t *testing.T) {
	locker := mountcom.NewResourceLocker()
	t.Run("Acquire", func(t *testing.T) { testAcquire(t, locker) })
}

func testAcquire(t *testing.T, locker mountcom.ResourceLock) {
	const (
		namespace = mountinter.NamespaceIPFS
		target    = "/lock/test/path"
		lType     = mountcom.LockDataWrite
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
