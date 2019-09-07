package fsnodes

import (
	"bytes"
	"strings"

	"github.com/djdv/p9/p9"
)

func ExampleRootIndex() {
	root, err := fsnodes.RootAttacher(ctx, coreAPI).Attach()
	_, file, err := root.Walk(strings.Split("ipfs/Qm.../subdir/file", "/"))
	_, _, err := file.Open(p9.ReadOnly)
	defer file.Close()
	_, err := file.ReadAt(byteBuffer, offset)
}

func ExampleIPFS() {
	ipfs, err := fsnodes.IPFSAttacher(ctx, coreAPI).Attach()
	_, file, err := ipfs.Walk(strings.Split("Qm.../subdir/file", "/"))
	_, _, err := file.Open(p9.ReadOnly)
	defer file.Close()
	_, err := file.ReadAt(byteBuffer, offset)
}

func ExamplePinFS() {
	ipfs, err := fsnodes.PinFSAttacher(ctx, coreAPI).Attach()
	_, dir, err := ipfs.Walk(nil)
	_, _, err := dir.Open(p9.ReadOnly)
	entries, err := dirClone.Readdir(offset, entryReturnCount)
}