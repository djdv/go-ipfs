package ipfscore

import (
	"context"
	"errors"
	gopath "path"
	"strings"

	"github.com/ipfs/go-ipfs/filesystem"
	fserrors "github.com/ipfs/go-ipfs/filesystem/errors"
	interfaceutils "github.com/ipfs/go-ipfs/filesystem/interface"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

var errNotImplemented = &interfaceutils.Error{
	Cause: errors.New("read only FS, modification operations are not implemented"),
	Type:  fserrors.InvalidOperation,
}

type coreInterface struct {
	ctx      context.Context
	core     interfaceutils.CoreExtender
	systemID filesystem.ID
}

func (ci *coreInterface) ID() filesystem.ID { return ci.systemID }

func NewInterface(ctx context.Context, core coreiface.CoreAPI, systemID filesystem.ID) filesystem.Interface {
	return &coreInterface{
		ctx:      ctx,
		core:     &interfaceutils.CoreExtended{CoreAPI: core},
		systemID: systemID,
	}
}

func (ci *coreInterface) joinRoot(path string) corepath.Path {
	return corepath.New(gopath.Join("/", strings.ToLower(ci.systemID.String()), path))
}

func (*coreInterface) Close() error { return nil }

func (*coreInterface) Rename(_, _ string) error { return errNotImplemented }
