//go:build bazilfuse && (windows || plan9 || netbsd || openbsd)
// +build bazilfuse
// +build windows plan9 netbsd openbsd

package bazil

import (
	"context"
	"fmt"

	"github.com/ipfs/go-ipfs/core"
	"github.com/ipfs/go-ipfs/filesystem"
	"github.com/ipfs/go-ipfs/filesystem/manager"
)

func NewBinder(context.Context, filesystem.ID, *core.IpfsNode, bool) (manager.Binder, error) {
	//return nil, errors.New("bazil Fuse support is not built into this binary")
	return new(unsupportedBinder), nil
}

type unsupportedBinder struct{}

func (*unsupportedBinder) Bind(context.Context, manager.Requests) manager.Responses {
	responses := make(chan manager.Response, 1)
	responses <- manager.Response{Error: fmt.Errorf("Bazil Fuse not supported in this build")}
	close(responses)
	return responses
}
