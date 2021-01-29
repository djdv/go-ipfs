package fscmds

import (
	"context"
	goerrors "errors"
	"fmt"
	"strings"

	fslock "github.com/ipfs/go-fs-lock"
	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs/core"
	"github.com/ipfs/go-ipfs/core/commands/cmdenv"
	"github.com/ipfs/go-ipfs/filesystem"
	"github.com/ipfs/go-ipfs/filesystem/manager"
	"github.com/ipfs/go-ipfs/filesystem/manager/errors"
)

const (
	MountParameter           = "mount"
	MountArgumentDescription = "Multiaddr style targets to bind with host. (/fuse/ipfs/path/ipfs)"
	listOptionKwd            = "list"
)

var Mount = &cmds.Command{
	Options: []cmds.Option{
		cmds.BoolOption(listOptionKwd, "l", "list active instances"), // TODO: constants
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("targets", false, true, MountArgumentDescription),
	},
	PreRun: mountPreRun,
	Run:    mountRun,
	PostRun: cmds.PostRunMap{
		cmds.CLI: mountPostRunCLI,
	},
	Encoders: cmds.Encoders,
	Helptext: cmds.HelpText{
		Tagline:          MountTagline,
		ShortDescription: mountDescWhatAndWhere,
		LongDescription:  mountDescWhatAndWhere + "\nExample:\n" + mountDescExample,
	},
	Type: manager.Response{},
}

// TODO: English pass; try to break apart code too, this is gross
// construct subcommand groups from supported API/ID pairs
// e.g. make these invocations equal
// 1) `ipfs mount /fuse/ipfs/path/mountpoint /fuse/ipfs/path/mountpoint2 ...
// 2) `ipfs mount fuse /ipfs/path/mountpoint /ipfs/path/mountpoint2 ...
// 3) `ipfs mount fuse ipfs /mountpoint /mountpoint2 ...
// allow things like `ipfs mount fuse -l` to list all fuse instances only, etc.
// shouldn't be too difficult to generate
// run re-executes `mount` with each arg prefixed `subreq.Args += api/id.String+arg`
func init() { registerSubcommands(Mount); return }

// TODO: simplify and document
// prefix arguments with constants to make the CLI experience a little nicer to use
// TODO: filtered --list + helptext (use some fmt tmpl)
func registerSubcommands(parent *cmds.Command) {
	template := &cmds.Command{
		Arguments: []cmds.Argument{
			cmds.StringArg("targets", false, true, MountArgumentDescription),
		},
		Run:      parent.Run,
		PostRun:  parent.PostRun,
		Encoders: parent.Encoders,
		Type:     parent.Type,
	}

	subcommands := make(map[string]*cmds.Command)
	for _, api := range []filesystem.API{
		filesystem.Fuse,
		filesystem.Plan9Protocol,
	} {
		hostName := api.String()
		subsystems := make(map[string]*cmds.Command)

		com := new(cmds.Command)
		*com = *template
		com.PreRun = func(request *cmds.Request, env cmds.Environment) error {
			if len(request.Arguments) == 0 {
				return fmt.Errorf("no arguments provided")
			}
			for i, arg := range request.Arguments {
				request.Arguments[i] = fmt.Sprintf("/%s/%s", hostName, strings.TrimPrefix(arg, "/"))
			}
			return parent.PreRun(request, env)
		}
		com.Subcommands = subsystems
		subcommands[hostName] = com

		for _, id := range []filesystem.ID{
			filesystem.IPFS,
			filesystem.IPNS,
		} {
			nodeName := id.String()
			com := new(cmds.Command)
			*com = *template
			com.PreRun = func(request *cmds.Request, env cmds.Environment) (err error) {
				if len(request.Arguments) == 0 {
					return fmt.Errorf("no arguments provided")
				}
				for i, arg := range request.Arguments {
					request.Arguments[i] = fmt.Sprintf("/%s/%s/path/%s", hostName, nodeName, strings.TrimPrefix(arg, "/"))
				}
				return parent.PreRun(request, env)
			}
			subsystems[nodeName] = com
		}
	}
	parent.Subcommands = subcommands
}

const postRunKey = "👻" // arbitrary index value that's smaller than its description
type mountExtra = cmds.Environment

func mountPreRun(request *cmds.Request, env cmds.Environment) (err error) {
	if len(request.Arguments) == 0 {
		request.Arguments = []string{
			"/fuse/ipfs",
			"/fuse/ipns",
		}
	}

	err = maybeSetPostRunExtra(request, env)
	return
}

// special handling for things like local-only requests
func maybeSetPostRunExtra(request *cmds.Request, env cmds.Environment) (err error) {
	// HACK: we need to do this, but properly
	// if the daemon already has the lock, don't do anything
	// otherwise, make the fsi and attach it to the node
	// figure out when/where our commands lock and do the check there instead of here
	// if we do it here we might not be able to guarantee who is holding the lock
	// and just assume it's the daemon (bad)
	var node *core.IpfsNode
	if node, err = cmdenv.GetNode(env); err != nil {
		if goerrors.As(err, new(fslock.LockedError)) {
			err = nil
		}
		return
	}
	// `ipfs daemon` will set up the fsi for us, and close it when it's done.
	// But if the daemon isn't running, it won't exist on the node.
	// So we spawn one that will close when this request is done (after PostRun returns).
	if !node.IsDaemon && node.FileSystem == nil {
		var isList bool // `--list` TODO: de-dupe
		if listFlag, provided := request.Options[listOptionKwd]; provided {
			if flag, isBool := listFlag.(bool); isBool {
				isList = flag
			} else {
				err = paramError(listOptionKwd, listFlag, isList)
				return
			}
		}

		if isList {
			err = cmds.Errorf(cmds.ErrNormal, "no active file system manager instances - nothing to list")
			return
		}

		var fsi manager.Interface
		if fsi, err = NewNodeInterface(request.Context, node); err != nil {
			err = fmt.Errorf("failed to construct file system interface: %w", err)
			return
		}

		node.FileSystem = fsi
		request.Root.Extra = request.Root.Extra.SetValue(postRunKey, env)
	}
	return
}

func mountRun(request *cmds.Request, emitter cmds.ResponseEmitter, env cmds.Environment) (err error) {
	var doList bool // `--list`
	if listFlag, provided := request.Options[listOptionKwd]; provided {
		if flag, isBool := listFlag.(bool); isBool {
			doList = flag
		} else {
			err = paramError(listOptionKwd, listFlag, doList)
			return
		}
	}

	defer func() {
		if err != nil {
			err = cmds.Errorf(cmds.ErrNormal, "%s", err)
		}
	}()

	var node *core.IpfsNode
	if node, err = cmdenv.GetNode(env); err != nil {
		err = fmt.Errorf("failed to get node instance from environment: %w", err)
		return
	}

	fsi := node.FileSystem

	ctx, cancel := context.WithCancel(request.Context)
	defer cancel()

	var responses manager.Responses
	errorStreams := make([]errors.Stream, 0, 2)
	if doList {
		responses = fsi.List(ctx)
	} else {
		requests, requestErrors := manager.ParseRequests(ctx, request.Arguments...)
		responses = fsi.Bind(ctx, requests)
		errorStreams = append(errorStreams, requestErrors)
	}

	responses = emitResponses(ctx, emitter, responses)
	errorStreams = append(errorStreams, responsesToErrors(ctx, responses))
	err = errors.WaitFor(ctx, errorStreams...)

	return
}

// TODO: hardcoded async stdout printing ruins our output :^(
// the logger gets a better error than the caller too...
// https://github.com/bazil/fuse/blob/371fbbdaa8987b715bdd21d6adc4c9b20155f748/mount_linux.go#L98-L99
// TODO: needs to be broken up more, clear split between foreground and background requests, as well as formatted vs encoded
// TODO: support `--enc`; if request encoding type is not "text", don't print extra messages or render the table
// just write responses directly to stdout
//
// formats responses for CLI/terminal displays
func mountPostRunCLI(response cmds.Response, emitter cmds.ResponseEmitter) (err error) {
	// NOTE: We expect CLI encoding to be text.
	// If it is, we'll format output for a terminal.
	// Otherwise, we'll pass values through to the emitter.
	// (which is likely: encoderMap(value) => stdout)

	var isList bool // `--list`
	if listFlag, provided := response.Request().Options[listOptionKwd]; provided {
		if flag, isBool := listFlag.(bool); isBool {
			isList = flag
		} else {
			err = paramError(listOptionKwd, listFlag, isList)
			return
		}
	}

	ctx := response.Request().Context
	if isList {
		err = emitList(ctx, emitter, response)
	} else {
		if err = emitBind(ctx, emitter, response); err != nil {
			return
		}
		extra, setInPreRun := response.Request().Root.Extra.GetValue(postRunKey)
		if setInPreRun {
			mountExtra, isExtra := extra.(mountExtra)
			if !isExtra {
				panic("extra value is wrong type") // TODO: handle for real
			}
			emitBindPostrun(response.Request(), emitter, mountExtra)
		}
	}
	return
}

func paramError(parameterName string, argument interface{}, expectedType interface{}) error {
	return cmds.Errorf(cmds.ErrClient,
		parameterName+" argument (%v) is type: %T, expecting type: %T",
		argument, argument, expectedType)
}
