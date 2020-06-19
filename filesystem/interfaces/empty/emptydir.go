package empty

import (
	"context"

	transform "github.com/ipfs/go-ipfs/filesystem"
)

var _ transform.Directory = (*emptyDir)(nil)

func OpenDir() *emptyDir { return new(emptyDir) }

type emptyDir struct{}

func (*emptyDir) Close() error { return nil }
func (*emptyDir) Reset() error { return nil }
func (*emptyDir) List(_ context.Context, _ uint64) <-chan transform.DirectoryEntry {
	ret := make(chan transform.DirectoryEntry)
	close(ret) // it had a good run but it's over now
	return ret
}
