package mountcmds

import (
	cmds "github.com/ipfs/go-ipfs-cmds"
	config "github.com/ipfs/go-ipfs-config"
	"github.com/ipfs/go-ipfs/filesystem/mount"
)

var DaemonOpts = []cmds.Option{
	cmds.BoolOption(daemonCmdMountKwd, cmdDaemonMountDesc),
	cmds.StringOption(daemonCmdProviderKwd, cmdDaemonDescPrefix+cmdProviderDesc),
	cmds.StringOption(daemonCmdNamespaceKwd, cmdDaemonDescPrefix+cmdNamespaceDesc),
	cmds.StringOption(daemonCmdTargetKwd, cmdDaemonDescPrefix+cmdPathDesc),
}

func ParseDaemonRequest(req *cmds.Request, nodeConf *config.Config) (mount.ProviderType, []mount.Request, error) {
	return parseRequest(daemonCmd, req, nodeConf)
}
