package overlay

import (
	"context"
	"strings"

	"github.com/hugelgupf/p9/p9"
	"github.com/ipfs/go-ipfs/core"
	"github.com/ipfs/go-ipfs/core/coreapi"
	"github.com/ipfs/go-ipfs/mount/providers/9P/filesystems"
	logging "github.com/ipfs/go-log"
)

func ExampleFile() {
	// obtain a CoreAPI instance however you like
	// in this example we'll just construct a temporary one
	ctx := context.TODO()
	node, _ := core.NewNode(ctx, &core.BuildCfg{
		Online:                      false,
		Permanent:                   false,
		DisableEncryptedConnections: true,
	})
	core, _ := coreapi.NewCoreAPI(node)

	// and just pass it to the `Attacher`, calling its `Attach` method
	// (with or without options)

	opts := []common.AttachOption{
		common.Logger(logging.Logger("🧊")),
		common.MFSRoot(node.FilesRoot),
	}

	//root, _ := Attacher(ctx, core).Attach()
	root, _ := Attacher(ctx, core, opts...).Attach()

	// this returns a reference to the root
	// which you can use to "walk" to other references
	// by passing a slice of path components
	_, file, _ := root.Walk(strings.Split("ipfs/Qm.../subdir/file", "/"))

	// and open them
	file.Open(p9.ReadOnly)
	defer file.Close()

	// etc.
	byteBuffer := make([]byte, 123)
	file.ReadAt(byteBuffer, 0)
}
