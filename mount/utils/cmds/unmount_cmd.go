package mountcmds

import (
	"errors"
	"fmt"
	"strings"

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

	Options: append(cmdSharedOpts, cmds.BoolOption(cmdUnmountAll, "a", "Unmount all instances.")),

	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) (err error) {
		defer res.Close()

		daemon, err := cmdenv.GetNode(env)
		if err != nil {
			cmds.EmitOnce(res, err)
			return err
		}

		if daemon.Mount == nil { // NOTE: this may be instantiated via `mount` or `daemon --mount`
			err := errors.New("No mount instances exist")
			cmds.EmitOnce(res, err)
			return err
		}

		nodeConf, err := cmdenv.GetConfig(env)
		if err != nil {
			cmds.EmitOnce(res, err)
			return err
		}

		if detachAll, ok := req.Options[cmdUnmountAll].(bool); ok && detachAll {
			daemon, err := cmdenv.GetNode(env)
			if err != nil {
				cmds.EmitOnce(res, err)
				return err
			}
			if daemon.Mount == nil {
				return errors.New("no instances")
			}
			whence := daemon.Mount.Where()
			var lastErr error
			for _, targets := range whence {
				for _, target := range targets {
					if lastErr = daemon.Mount.Detach(target); lastErr != nil {
						res.Emit(fmt.Sprintf("could not detach %q: %s", target, lastErr))
					}
				}
			}
			// TODO: prettify targets
			cmds.EmitOnce(res, fmt.Sprintf("unmounted: %v", whence))
			return lastErr
		}

		_, targets, err := parseRequest(mountCmd, req, nodeConf)
		if err != nil {
			cmds.EmitOnce(res, err)
			return err
		}

		var (
			retErrString strings.Builder
			failed       bool
		)

		retErrString.WriteString("failed to detach: ")

		for _, pair := range targets {
			if err := daemon.Mount.Detach(pair.Target); err != nil {
				failed = true
				retErrString.WriteString(fmt.Sprintf("{\"%s\", error: %s} ", pair.Target, err.Error()))
			}
		}
		if failed {
			// TODO: how about not putting the extra space in the first place 👀
			err := errors.New(strings.TrimPrefix(retErrString.String(), " "))
			cmds.EmitOnce(res, err)
			return err
		}

		// TODO: print response if nothing is mounted, but don't error
		return nil
	},
}
