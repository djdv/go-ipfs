package fscmds

import (
	"encoding/json"

	"github.com/multiformats/go-multiaddr"
)

// file system requests are just multiaddrs
// `/fuse/ipfs/path/ipfs` effectively means fuse.mount(ipfs, "/ipfs")
type fsRequest struct{ multiaddr.Multiaddr }

// NOTE: these methods exist to allocate a concrete `multiaddr.Multiaddr`
// using the same conventions as their inherited type's `MarshalX` methods
// TODO: if there's a better way to do this, we should do that.

func (fsr *fsRequest) UnmarshalJSON(data []byte) (err error) {
	var maddr string
	if err = json.Unmarshal(data, &maddr); err != nil {
		return
	}

	fsr.Multiaddr, err = multiaddr.NewMultiaddr(maddr)
	return
}

func (fsr *fsRequest) UnmarshalBinary(data []byte) (err error) {
	fsr.Multiaddr, err = multiaddr.NewMultiaddrBytes(data)
	return
}

func (fsr *fsRequest) UnmarshalText(data []byte) (err error) {
	fsr.Multiaddr, err = multiaddr.NewMultiaddr(string(data))
	return
}

func parseCommandLine(requests []string) (maddrs []multiaddr.Multiaddr, err error) {
	var ma multiaddr.Multiaddr
	for _, maddr := range requests {
		if ma, err = multiaddr.NewMultiaddr(maddr); err != nil {
			return
		}
		maddrs = append(maddrs, ma)
	}

	return
}
