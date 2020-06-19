package ipfscore

import (
	"context"
	"errors"
	gopath "path"
	"strings"

	transform "github.com/ipfs/go-ipfs/filesystem"
	transcom "github.com/ipfs/go-ipfs/filesystem/interfaces"
	mountinter "github.com/ipfs/go-ipfs/filesystem/mount"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

var errNotImplemented = &transcom.Error{
	Cause: errors.New("read only FS, modification operations are not implemented"),
	Type:  transform.ErrorInvalidOperation,
}

var _ transform.Interface = (*coreInterface)(nil)

type coreInterface struct {
	ctx       context.Context
	core      transcom.CoreExtender
	namespace mountinter.Namespace
}

var _ transform.Interface = (*coreInterface)(nil)

func NewInterface(ctx context.Context, core coreiface.CoreAPI, namespace mountinter.Namespace) transform.Interface {
	return &coreInterface{
		ctx:       ctx,
		core:      &transcom.CoreExtended{core},
		namespace: namespace,
	}
}

func (ci *coreInterface) joinRoot(path string) corepath.Path {
	return corepath.New(gopath.Join("/", strings.ToLower(ci.namespace.String()), path))
}

func (*coreInterface) Close() error { return nil }

func (*coreInterface) Rename(_, _ string) error { return errNotImplemented }
