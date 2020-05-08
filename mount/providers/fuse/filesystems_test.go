package mountfuse

import (
	"context"
	"math"
	"testing"
)

type fileHandle = uint64
type errNo = int

// implementation detail: this value is what the fuse library passes to us for anonymous requests (like Getattr)
// we use this same value as the erronious handle value
// (for non-anonymous requests; i.e. returned from a failed Open call, checked in Read and reported as an invalid handle)
// despite being the same value, they are semantically separate depending on the context
const anonymousRequestHandle = fileHandle(math.MaxUint64)

func TestAll(t *testing.T) {
	_, testEnv, node, core, unwind := generateTestEnv(t)
	defer node.Close()
	t.Cleanup(unwind)

	ctx := context.TODO()

	t.Run("IPFS", func(t *testing.T) { testIPFS(ctx, t, testEnv, core) })
	/* TODO
	t.Run("IPNS", func(t *testing.T) { testIPNS(ctx, t, env, iEnv, core) })
	t.Run("FilesAPI", func(t *testing.T) { testMFS(ctx, t, env, iEnv, core) })
	t.Run("PinFS", func(t *testing.T) { testPinFS(ctx, t, env, iEnv, core) })
	t.Run("KeyFS", func(t *testing.T) { testKeyFS(ctx, t, env, iEnv, core) })
	t.Run("FS overlay", func(t *testing.T) { testOverlay(ctx, t, env, iEnv, core) })
	*/
}
