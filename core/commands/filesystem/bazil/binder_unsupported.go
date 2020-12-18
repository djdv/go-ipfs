//go:build bazilfuse && (windows || plan9 || netbsd || openbsd)
// +build bazilfuse
// +build windows plan9 netbsd openbsd

package bazil

import (
	"context"
	"errors"

	"github.com/ipfs/go-ipfs/core"
	"github.com/ipfs/go-ipfs/filesystem"
	"github.com/ipfs/go-ipfs/filesystem/manager"
)

func NewBinder(context.Context, filesystem.ID, *core.IpfsNode, bool) (manager.Binder, error) {
	return nil, errors.New("bazil Fuse support not built into this binary")
}
