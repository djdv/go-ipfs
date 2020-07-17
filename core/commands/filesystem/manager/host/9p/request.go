package p9fsp

import (
	"fmt"
	"os"
	gopath "path"
	"path/filepath"
	"strings"

	config "github.com/ipfs/go-ipfs-config"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"
	"github.com/ipfs/go-ipfs/filesystem"
	"github.com/multiformats/go-multiaddr"
)

type Request struct {
	ListenAddr string
	HostPath   string
}

func (r Request) Arguments() []string { return []string{r.ListenAddr} }
func (r Request) String() string {
	var sb strings.Builder
	sb.WriteString(gopath.Join("/",
		host.SocketNamespace,
		r.ListenAddr,
	))

	if r.HostPath != "" {
		sb.WriteString(gopath.Join("/",
			host.PathNamespace,
			r.HostPath,
		))
	}

	return sb.String()
}

func ParseRequest(sysID filesystem.ID, target string) (host.Request, error) {
	var err error

	// we allow templating unix domain socket maddrs, so check for those and expand them here
	if strings.HasPrefix(target, "/unix") {
		target, err = stabilizeUnixPath(target)
		if err != nil {
			return nil, err
		}
	}

	var request Request

	// requests may be for a host path or a listener
	// if the target is a listener, parse it and return
	if maddr, err := multiaddr.NewMultiaddr(target); err == nil {
		request.ListenAddr = maddr.String()
		return request, nil
	}

	// if the Request is for a path
	// provide a listening address for the server and client to use
	request.HostPath = target
	request.ListenAddr, err = stabilizeUnixPath(fmt.Sprintf("/unix/$IPFS_HOME/9p.%s.sock", sysID.String()))
	if err != nil {
		return request, err
	}

	return request, err
}

// TODO: templateRoot = (*nix) `/` || (NT) ${CurDrive}:\ || (any others)...
const templateHome = "IPFS_HOME"

func stabilizeUnixPath(maString string) (string, error) {
	templateValueRepoPath, err := config.PathRoot()
	if err != nil {
		return "", err
	}

	if !filepath.IsAbs(templateValueRepoPath) { // stabilize root path
		absRepo, err := filepath.Abs(templateValueRepoPath)
		if err != nil {
			return "", err
		}
		templateValueRepoPath = absRepo
	}

	// expand templates

	// NOTE: the literal parsing and use of `/` is interned
	// we don't want to treat this like a file system path, it is specifically a multiaddr string
	// this prevents the template from expanding to double slashed paths like `/unix//home/...` on *nix systems
	// but allow it to expand to `/unix/C:\Users\...` on NT, which is the valid form for the maddr target value
	templateValueRepoPath = strings.TrimPrefix(templateValueRepoPath, "/")

	// only expand documented template keys, not everything
	return os.Expand(maString, func(key string) string {
		return (map[string]string{
			templateHome: templateValueRepoPath,
		})[key]
	}), nil
}
