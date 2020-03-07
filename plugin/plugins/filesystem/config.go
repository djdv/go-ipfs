package filesystem

import (
	"fmt"

	config "github.com/ipfs/go-ipfs-config"
	serialize "github.com/ipfs/go-ipfs-config/serialize"
	plugin "github.com/ipfs/go-ipfs/plugin"
	"github.com/ipfs/go-ipfs/repo/common"
	"github.com/mitchellh/mapstructure"
)

const (
	defaultService = "9p" // (currently 9P2000.L)
	sockName       = "filesystem." + defaultService + ".sock"

	tmplHome = "IPFS_HOME"

	selectorBase = "Plugins.Plugins.filesystem.Config"
	selector9p   = selectorBase + ".Service.9p"
)

type fsPluginConfig struct { // NOTE: unstable/experimental
	// addresses for file system servers and clients
	//e.g. "9p":"/ip4/localhost/tcp/564", "fuse":"/mountpoint", ...
	Service map[string]string
}

func defaultConfig() *fsPluginConfig {
	return &fsPluginConfig{
		map[string]string{
			defaultService: fmt.Sprintf("/unix/${%s}/%s", tmplHome, sockName),
		},
	}
}

func loadPluginConfig(env *plugin.Environment) (*fsPluginConfig, error) {
	// if config was provided, try to use it
	if env.Config != nil && env.Config != (*fsPluginConfig)(nil) {
		// even if the interface is not nil, make sure it's data segment does not point to a nil struct
		// e.g. var bad *fsPluginConfig; env.Config = bad
		// makes for env.Config == nil == false

		// if env is populated, the node already parsed its full config
		// and passed us the plugins portion for us to parse ourselves
		cfg := &fsPluginConfig{}
		if err := mapstructure.Decode(env.Config, cfg); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	// otherwise we try to initalize with defaults
	conf := defaultConfig()

	return conf, nil
}

/* TODO: there's no obvious time to save the plugin's config to disk
we can't do it during:
1) plugin.Init() since the config file may not exist when we're called.
2) plugin.Start()|.Close() doesn't make much sense, it's not related to the method names and we'd be (re)writing the file when we don't need to anyway.

We should probably expose this function via the FS and document it so users can trigger it on demand:
	/config
		/show => string: $ConfigContents
		/save => either an int:(0, 1), a string:("success", "failed: err"), or both seperated by a newline
*/
func savePluginConfig(pluginConf *fsPluginConfig) error {
	confPath, err := config.Filename("")
	if err != nil {
		return err
	}

	var mapConf map[string]interface{}
	if err := serialize.ReadConfigFile(confPath, &mapConf); err != nil {
		return err
	}

	if err := common.MapSetKV(mapConf, selectorBase, pluginConf); err != nil {
		return err
	}

	finalConf, err := config.FromMap(mapConf)
	if err != nil {
		return err
	}

	if err := serialize.WriteConfigFile(confPath, finalConf); err != nil {
		return err
	}

	return nil
}
