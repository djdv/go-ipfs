package fuse

import (
	gopath "path"
	"strings"

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"
)

type Request struct {
	HostPath string
	FuseArgs []string
}

func (r Request) Arguments() []string { return r.FuseArgs }
func (r Request) String() string {
	hostPath := r.HostPath
	if hostPath == "" { // TODO: this is WinFSP specific and needs to be more explicit about that
		hostPath = extractUNCArg(r.FuseArgs)
	}

	return gopath.Join("/",
		host.PathNamespace,
		hostPath,
	)
}

func extractUNCArg(args []string) string {
	const uncArgPrefix = `--VolumePrefix=`
	for _, arg := range args {
		if strings.HasPrefix(arg, uncArgPrefix) {
			return `\` + strings.TrimPrefix(arg, uncArgPrefix)
		}
	}
	panic("empty host path and no path in args")
}
