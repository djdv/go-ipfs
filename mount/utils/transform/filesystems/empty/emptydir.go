package empty

import (
	"context"
	"os"

	"github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-ipfs/mount/utils/transform"
)

var (
	_ transform.Directory      = (*emptyDir)(nil)
	_ transform.DirectoryState = (*emptyDir)(nil)
)

type emptyDir struct{}

func OpenDir() *emptyDir                                                          { return new(emptyDir) }
func (ed *emptyDir) Readdir(_ context.Context, _ uint64) transform.DirectoryState { return ed }
func (*emptyDir) Close() error                                                    { return nil }
func (*emptyDir) To9P(_ uint32) (p9.Dirents, error)                               { return nil, nil }
func (*emptyDir) ToGo() ([]os.FileInfo, error)                                    { return nil, nil }
func (*emptyDir) ToGoC(dirChan chan os.FileInfo) (<-chan os.FileInfo, error) {
	if dirChan == nil {
		dirChan = make(chan os.FileInfo)
	}
	close(dirChan)

	return dirChan, nil
}
func (*emptyDir) ToFuse() (<-chan transform.FuseStatGroup, error) {
	dirChan := make(chan transform.FuseStatGroup)
	close(dirChan)
	return dirChan, nil
}
