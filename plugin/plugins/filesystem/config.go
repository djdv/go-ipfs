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

type Config struct { // NOTE: unstable/experimental
	// addresses for file system servers and clients
	//e.g. "9p":"/ip4/localhost/tcp/564", "fuse":"/mountpoint", ...
	Service map[string]string
}

func defaultConfig() *Config {
	return &Config{
		map[string]string{
			defaultService: fmt.Sprintf("/unix/${%s}/%s", tmplHome, sockName),
		},
	}
}

func loadPluginConfig(env *plugin.Environment) (*Config, error) {
	if env.Config != nil && env.Config != (*Config)(nil) {
		// If env is populated, the node already parsed its config for us
		// We then parse the plugin portion given to us
		cfg := &Config{}
		if err := mapstructure.Decode(env.Config, cfg); err != nil {
			return nil, err
		}
		return cfg, nil
	}

	// otherwise we try to initalize with defaults
	conf := defaultConfig()
	if err := savePluginConfig(conf); err != nil {
		return nil, err
	}

	return conf, nil
}

func savePluginConfig(pluginConf *Config) error {
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
