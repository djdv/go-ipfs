package common

import corepath "github.com/ipfs/interface-go-ipfs-core/path"

func ExampleRootPath() {
	VirtualRootCid := RootPath("/ipfs").Cid()
	iPath := corepath.Join(RootPath("/ipfs"), "Qm...")
}
