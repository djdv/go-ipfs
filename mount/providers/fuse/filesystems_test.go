package mountfuse

import (
	"context"
	"testing"
)

type fileHandle = uint64

func TestAll(t *testing.T) {
	env, iEnv, node, core, unwind := generateTestEnv(t)
	defer node.Close()
	t.Cleanup(unwind)

	ctx := context.TODO()

	t.Run("IPFS", func(t *testing.T) { testIPFS(ctx, t, env, iEnv, core) })
	/* TODO
	t.Run("IPNS", func(t *testing.T) { testIPNS(ctx, t, env, iEnv, core) })
	t.Run("FilesAPI", func(t *testing.T) { testMFS(ctx, t, env, iEnv, core) })
	t.Run("PinFS", func(t *testing.T) { testPinFS(ctx, t, env, iEnv, core) })
	t.Run("KeyFS", func(t *testing.T) { testKeyFS(ctx, t, env, iEnv, core) })
	t.Run("FS overlay", func(t *testing.T) { testOverlay(ctx, t, env, iEnv, core) })
	*/
}
