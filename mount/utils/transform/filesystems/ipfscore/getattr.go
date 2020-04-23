package ipfscore

import (
	"context"

	"github.com/ipfs/go-ipfs/mount/utils/transform"
	ipld "github.com/ipfs/go-ipfs/mount/utils/transform/filesystems/ipld"
	coreiface "github.com/ipfs/interface-go-ipfs-core"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

// TODO: [investigate] [b6150f2f-8689-4e60-a605-fd40c826c32d]
// GetAttr resolves an IPFS API path and returns the attr, filled attr members, and error associated with the path
func GetAttr(ctx context.Context, path corepath.Path, core coreiface.CoreAPI, req transform.IPFSStatRequest) (*transform.IPFSStat, transform.IPFSStatRequest, error) {
	ipldNode, err := core.ResolveNode(ctx, path)
	if err != nil {
		return nil, transform.IPFSStatRequest{}, err
	}

	return ipld.GetAttr(ctx, ipldNode, req)
}
