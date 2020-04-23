package empty

import (
	"github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
)

var (
	_ transform.Directory      = (*emptyDir)(nil)
	_ transform.DirectoryState = (*emptyDir)(nil)
)

type emptyDir struct {
}

func (*emptyDir) To9P() (p9.Dirents, error) {
	return nil, nil
}

func (*emptyDir) ToFuse() (<-chan transform.FuseStatGroup, error) {
	dirChan := make(chan transform.FuseStatGroup)
	close(dirChan)
	return dirChan, nil
}

func (ed *emptyDir) Readdir(_, _ uint64) transform.DirectoryState { return ed }

func OpenDir() *emptyDir {
	return new(emptyDir)
}

func (*emptyDir) Close() error {
	return nil
}
