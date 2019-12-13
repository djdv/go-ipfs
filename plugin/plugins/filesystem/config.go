package filesystem

import (
	"errors"
	"fmt"

	config "github.com/ipfs/go-ipfs-config"
	serialize "github.com/ipfs/go-ipfs-config/serialize"
	"github.com/ipfs/go-ipfs/repo/common"
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
	//e.g. "9p":"/ip4/localhost/tcp/564", "fuse":"/mountpoint", "đ":"/rabbit-hutch/glenda", ...
	Service map[string]string
}

func defaultConfig() *Config {
	return &Config{
		map[string]string{
			defaultService: fmt.Sprintf("/unix/${%s}/%s", tmplHome, sockName),
		},
	}
}

func saveConfig(nodeConf map[string]interface{}, pluginConf *Config) error {
	confPath, err := config.Filename(config.DefaultConfigFile)
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
	return errors.New("NIY")
}
