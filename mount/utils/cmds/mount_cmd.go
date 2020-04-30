package mountcmds

import (
	"fmt"
	"strings"

	"github.com/ipfs/go-ipfs/core/commands/cmdenv"
	"github.com/ipfs/go-ipfs/core/coreapi"
	mountcon "github.com/ipfs/go-ipfs/mount/conductors/ipfs-core"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"

	cmds "github.com/ipfs/go-ipfs-cmds"
)

const cmdListInstances = "list"

var MountCmd = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Mounts IPFS to the filesystem.",
		ShortDescription: `
		TODO: change this text
Mount IPFS at a read-only mountpoint on the OS (default: /ipfs and /ipns).
All IPFS objects will be accessible under that directory. Note that the
root will not be listable, as it is virtual. Access known paths directly.

You may have to create /ipfs and /ipns before using 'ipfs mount':

> sudo mkdir /ipfs /ipns
> sudo chown $(whoami) /ipfs /ipns
> ipfs daemon &
> ipfs mount
`,
		LongDescription: `
		TODO: change this text
Mount IPFS at a read-only mountpoint on the OS. The default, /ipfs and /ipns,
are set in the configuration file, but can be overriden by the options.
All IPFS objects will be accessible under this directory. Note that the
root will not be listable, as it is virtual. Access known paths directly.

You may have to create /ipfs and /ipns before using 'ipfs mount':

> sudo mkdir /ipfs /ipns
> sudo chown $(whoami) /ipfs /ipns
> ipfs daemon &
> ipfs mount

Example:

# setup
> mkdir foo
> echo "baz" > foo/bar
> ipfs add -r foo
added QmWLdkp93sNxGRjnFHPaYg8tCQ35NBY3XPn6KiETd3Z4WR foo/bar
added QmSh5e7S6fdcu75LAbXNZAFY2nGyZUJXyLCJDvn2zRkWyC foo
> ipfs ls QmSh5e7S6fdcu75LAbXNZAFY2nGyZUJXyLCJDvn2zRkWyC
QmWLdkp93sNxGRjnFHPaYg8tCQ35NBY3XPn6KiETd3Z4WR 12 bar
> ipfs cat QmWLdkp93sNxGRjnFHPaYg8tCQ35NBY3XPn6KiETd3Z4WR
baz

# mount
> ipfs daemon &
> ipfs mount
IPFS mounted at: /ipfs
IPNS mounted at: /ipns
> cd /ipfs/QmSh5e7S6fdcu75LAbXNZAFY2nGyZUJXyLCJDvn2zRkWyC
> ls
bar
> cat bar
baz
> cat /ipfs/QmSh5e7S6fdcu75LAbXNZAFY2nGyZUJXyLCJDvn2zRkWyC/bar
baz
> cat /ipfs/QmWLdkp93sNxGRjnFHPaYg8tCQ35NBY3XPn6KiETd3Z4WR
baz
`,
	},
	Options: append(cmdSharedOpts,
		cmds.BoolOption(cmdListInstances, "l", "List mounted instances.")),

	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) (err error) {
		defer res.Close()

		daemon, err := cmdenv.GetNode(env)
		if err != nil {
			cmds.EmitOnce(res, err)
			return err
		}

		if daemon.Mount == nil { // NOTE: this may be instantiated via `mount` or `daemon --mount`
			coreAPI, err := coreapi.NewCoreAPI(daemon)
			if err != nil {
				cmds.EmitOnce(res, err)
				return err
			}

			var cOps []mountcon.Option
			if mroot := daemon.FilesRoot; mroot != nil {
				cOps = append(cOps, mountcon.WithFilesAPIRoot(*mroot))
			}

			if !daemon.IsDaemon {
				cOps = append(cOps, mountcon.InForeground(true))
			}

			daemon.Mount = mountcon.NewConductor(daemon.Context(), coreAPI, cOps...)
		}

		if listArg, ok := req.Options[cmdListInstances].(bool); ok && listArg {
			cmds.EmitOnce(res, prettifyWhere(daemon.Mount.Where()))
			return nil
		}

		nodeConf, err := cmdenv.GetConfig(env)
		if err != nil {
			cmds.EmitOnce(res, err)
			return err
		}

		provider, targets, err := parseRequest(mountCmd, req, nodeConf)
		if err != nil {
			cmds.EmitOnce(res, err)
			return err
		}

		if err := MountNode(res, daemon, provider, targets); err != nil {
			// FIXME: for some reason EmitOnce isn't emitting the proper value
			// it just returns `{}` on the cli...
			fmt.Println(err)
			//HACK^
			cmds.EmitOnce(res, err)
			return err
		}
		return nil
	},
}

func prettifyWhere(m map[mountinter.ProviderType][]string) string {
	var s strings.Builder
	for prov, targets := range m {
		s.WriteString(prov.String() + ": [")
		tEnd := len(targets) - 1
		for i, targ := range targets {
			s.WriteString(targ)
			if i != tEnd {
				s.WriteString(", ")
			}
		}
		s.WriteString("]")
	}
	if s.Len() == 0 {
		return "No mounts are active"
	}
	return s.String()
}
