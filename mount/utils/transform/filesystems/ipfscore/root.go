package ipfscore

import (
	"fmt"
	gopath "path"

	mountinter "github.com/ipfs/go-ipfs/mount/interface"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

// TODO: [investigate] [b6150f2f-8689-4e60-a605-fd40c826c32d]
func joinRoot(ns mountinter.Namespace, path string) (corepath.Path, error) {
	var rootPath string
	switch ns {
	default:
		return nil, fmt.Errorf("unsupported namespace: %s", ns.String())
	case mountinter.NamespaceIPFS:
		rootPath = "/ipfs"
	case mountinter.NamespaceIPNS:
		rootPath = "/ipns"
	case mountinter.NamespaceCore:
		rootPath = "/ipld"
	}
	return corepath.New(gopath.Join(rootPath, path)), nil
}
