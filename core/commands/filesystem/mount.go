package fscmds

import (
	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs/core/commands/cmdenv"
	"github.com/ipfs/go-ipfs/filesystem"
	"github.com/ipfs/go-ipfs/fuse/mount"
	nodeMount "github.com/ipfs/go-ipfs/fuse/node"
)

const (
	MountParameter           = "mount"
	MountArgumentDescription = "Multiaddr style targets to bind with host. (/fuse/ipfs/path/ipfs)"
)

var Mount = &cmds.Command{
	Arguments: []cmds.Argument{
		cmds.StringArg("targets", false, true, MountArgumentDescription),
	},
	Run: mountRun,
	Helptext: cmds.HelpText{
		Tagline:          MountTagline,
		ShortDescription: mountDescWhatAndWhere,
		LongDescription:  mountDescWhatAndWhere + "\nExample:\n" + mountDescExample,
	},
	Type:     &fsRequest{},
	Encoders: cmds.Encoders,
	PostRun: cmds.PostRunMap{
		cmds.CLI:          runFormatCLI,
		daemonPostRunType: runFormatCLI,
	},
}

func mountRun(req *cmds.Request, re cmds.ResponseEmitter, env cmds.Environment) error {
	// check if node is mountable
	node, err := cmdenv.GetNode(env)
	if err != nil {
		return cmds.Errorf(cmds.ErrNormal, "failed to get node instance from environment: %v", err)
	}

	if !node.IsOnline {
		return cmds.Errorf(cmds.ErrClient, "mount is not currently supported in offline mode")
	}

	// parse arguments (if any)
	requests, err := parseCommandLine(req.Arguments)
	if err != nil {
		return cmds.Errorf(cmds.ErrClient, "failed to parse mount arguments: %v", err)
	}

	// legacy note: current implementation has fixed expectations
	// if more than 1 argument is provided per API, the last argument's value is used
	// this will not be relevant in the new implementation
	var ipfsPath, ipnsPath string
	for _, bindRequest := range requests {
		mountPoint, err := bindRequest.ValueForProtocol(int(filesystem.PathProtocol))
		if err != nil {
			return cmds.Errorf(cmds.ErrClient, "path not provided in argument: %v", err)
		}

		nodeAPI, err := bindRequest.ValueForProtocol(int(filesystem.Fuse))
		if err != nil {
			return cmds.Errorf(cmds.ErrClient, "file system API not provided in argument: %v", err)
		}

		switch nodeAPI {
		case filesystem.IPFS.String():
			ipfsPath = mountPoint
		case filesystem.IPNS.String():
			ipnsPath = mountPoint
		default:
			return cmds.Errorf(cmds.ErrImplementation, "node API %v is not currently supported", nodeAPI)
		}
	}

	// fallback to config for fill-in values
	if ipfsPath == "" || ipnsPath == "" {
		nodeConf, err := cmdenv.GetConfig(env)
		if err != nil {
			return cmds.Errorf(cmds.ErrNormal, "failed to get node config from environment: %v", err)
		}

		if ipfsPath == "" {
			ipfsPath = nodeConf.Mounts.IPFS
		}

		if ipnsPath == "" {
			ipnsPath = nodeConf.Mounts.IPNS
		}
	}

	// NOTE: values sent must match cmd emitter's type: `cmd.Command.Type`
	emitterChan := make(chan interface{})

	// parse the responses from
	emitResponse := func(id filesystem.ID, instance mount.Mount) error {
		if instance == nil || !instance.IsActive() {
			return nil
		}

		ma, err := filesystem.NewFuse(id, instance.MountPoint())
		if err != nil {
			return err
		}

		emitterChan <- &fsRequest{ma}
		return nil
	}

	go func() {
		defer close(emitterChan) // close emitter when done

		err := nodeMount.Mount(node, ipfsPath, ipnsPath) // try to mount the requests we got
		if err != nil {
			return
		}
		for _, pair := range []struct { // for each supported mountpoint
			filesystem.ID
			mount.Mount
		}{
			{filesystem.IPFS, node.Mounts.Ipfs},
			{filesystem.IPNS, node.Mounts.Ipns},
		} {
			if err = emitResponse(pair.ID, pair.Mount); err != nil { // emit their current status
				return
			}
		}
	}()

	return cmds.EmitChan(re, emitterChan)
}
