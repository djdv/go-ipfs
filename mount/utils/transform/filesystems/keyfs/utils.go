package keyfs

import (
	"context"

	coreiface "github.com/ipfs/interface-go-ipfs-core"
	coreoptions "github.com/ipfs/interface-go-ipfs-core/options"
	corepath "github.com/ipfs/interface-go-ipfs-core/path"
)

// we publish offline just to initialize the key in the node's context
// the world update is not our concern; we just want to be fast locally
// the caller should make a globabl broadcast if they want to sync with the wired
func localPublish(ctx context.Context, core coreiface.CoreAPI, keyName string, target corepath.Path) error {
	oAPI, err := core.WithOptions(coreoptions.Api.Offline(true))
	if err != nil {
		return err
	}

	_, err = oAPI.Name().Publish(ctx, target, coreoptions.Name.Key(keyName), coreoptions.Name.AllowOffline(true))
	return err
}
