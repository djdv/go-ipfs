package mountcmds

import (
	"errors"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs/core/commands/cmdenv"
)

const cmdUnmountAll = "all"

var UnmountCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Removes IPFS mountpoints from the filesystem.",
		ShortDescription: `
		TODO: replace this text :^)
`,
		LongDescription: `
		TODO: replace this text :^)
`,
	},

	Options: append(cmdSharedOpts,
		cmds.BoolOption(cmdUnmountAll, "a", "Unmount all instances.")),

	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) (err error) {
		defer res.Close()

		daemon, err := cmdenv.GetNode(env)
		if err != nil {
			cmds.EmitOnce(res, err)
			return err
		}

		if daemon.Mount == nil { // NOTE: this may be instantiated via `mount` or `daemon --mount`
			err := errors.New("no mount instances exist")
			cmds.EmitOnce(res, err)
			return err
		}

		nodeConf, err := cmdenv.GetConfig(env)
		if err != nil {
			cmds.EmitOnce(res, err)
			return err
		}

		if detachArg, ok := req.Options[cmdUnmountAll].(bool); ok && detachArg {
			return detachAll(res, env)
		}

		provider, requests, err := parseRequest(mountCmd, req, nodeConf)
		if err != nil {
			cmds.EmitOnce(res, err)
			return err
		}

		// TODO: print response if nothing is mounted, but don't error

		err = daemon.Mount.Detach(provider, requests...)
		if err != nil {
			cmds.EmitOnce(res, err)
		}
		return err
	},
}

//FIXME: quick and lazy port, we should do this properly; old method below
func detachAll(res cmds.ResponseEmitter, env cmds.Environment) error {
	daemon, err := cmdenv.GetNode(env)
	if err != nil {
		cmds.EmitOnce(res, err)
		return err
	}
	if daemon.Mount == nil {
		return errors.New("no instances")
	}
	return daemon.Mount.Close()
}

/*
func detachAll(res cmds.ResponseEmitter, env cmds.Environment) error {
	daemon, err := cmdenv.GetNode(env)
	if err != nil {
		cmds.EmitOnce(res, err)
		return err
	}
	if daemon.Mount == nil {
		return errors.New("no instances")
	}

	whence := daemon.Mount.List()
	var lastErr error
	for _, targets := range whence {
		for _, target := range targets {
			if lastErr = daemon.Mount.Detach(target.Target); lastErr != nil {
				res.Emit(fmt.Sprintf("could not detach \"%s\": %s", target, lastErr))
			}
			res.Emit(fmt.Sprintf("detached \"%s\"", target))
		}
	}

	// TODO: prettify targets
	cmds.EmitOnce(res, fmt.Sprintf("unmounted: %v", prettifyWhere(whence)))
	return lastErr
}
*/
