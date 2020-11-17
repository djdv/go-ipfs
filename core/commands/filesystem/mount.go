package fscmds

import (
	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs/core/commands/cmdenv"
	nodeMount "github.com/ipfs/go-ipfs/fuse/node"
)

var Mount = &cmds.Command{
	Run: mountRun,
	Helptext: cmds.HelpText{
		Tagline:          mountTagline,
		ShortDescription: mountDescWhatAndWhere,
		LongDescription:  mountDescWhatAndWhere + "\nExample:\n" + mountDescExample,
	},
	Options: baseOpts,
	Type:    &Response{},
	Encoders: cmds.EncoderMap{ // TODO: pull in the definitions for these when the format parsing is implemented
		cmds.Text: cmds.MakeEncoder(encodeText),
		cmds.JSON: cmds.MakeEncoder(encodeJSON),
	},
}

func mountRun(req *cmds.Request, re cmds.ResponseEmitter, env cmds.Environment) error {
	node, err := cmdenv.GetNode(env)
	if err != nil {
		return cmds.Errorf(cmds.ErrNormal, "failed to get node instance from environment: %v", err)
	}

	if !node.IsOnline {
		return cmds.Errorf(cmds.ErrClient, "mount is not currently supported in offline mode")
	}

	nodeConf, err := cmdenv.GetConfig(env)
	if err != nil {
		return cmds.Errorf(cmds.ErrNormal, "failed to get node config from environment: %v", err)
	}

	responses := make(chan interface{}, 1) // NOTE: value must match `cmd.Command.Type`
	// ^ responses := make(chan Response, 1) // cmds lib needs it to be interface{} here

	ipfsPath, ipnsPath, err := parseRequest(req, nodeConf.Mounts)
	if ipfsPath == "" && ipnsPath == "" { // XXX: this is a quick hack; we're changing this section later
		return err
	}

	go func() { // emit responses to the requester
		defer close(responses)
		// TODO: lint responses <- Response{Info: "binding to host..."}

		if err := nodeMount.Mount(node, ipfsPath, ipnsPath); err != nil {
			responses <- Response{Error: err}
			return
		}

		if node.Mounts.Ipfs != nil && node.Mounts.Ipfs.IsActive() {
			responses <- Response{Info: "IPFS mounted at: " + node.Mounts.Ipfs.MountPoint()}
		}

		if node.Mounts.Ipns != nil && node.Mounts.Ipns.IsActive() {
			responses <- Response{Info: "IPNS mounted at: " + node.Mounts.Ipns.MountPoint()}
		}
	}()

	return re.Emit(responses)
}
