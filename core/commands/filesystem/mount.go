package fscmds

import (
	"context"
	"log"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs/core"
	"github.com/ipfs/go-ipfs/core/commands/cmdenv"
	"github.com/ipfs/go-ipfs/filesystem/manager"
	"github.com/ipfs/go-ipfs/filesystem/manager/errors"
)

const (
	MountParameter           = "mount"
	MountArgumentDescription = "Multiaddr style targets to bind with host. (/fuse/ipfs/path/ipfs)"
	listOptionKwd            = "list"
)

// TODO: in init() construct subcommand groups from supported API/ID pairs
// e.g. make this work and be equal
// 1) `ipfs mount /fuse/ipfs/path/mountpoint /fuse/ipfs/mountpoint2 ...
// 2) `ipfs mount fuse  /ipfs/path/mountpoint /ipfs/path/mountpoint2 ...
// 3) `ipfs mount fuse ipfs /path/mountpoint /path/mountpoint2 ...
// allow things like `ipfs mount fuse -l` to list all fuse instances only, etc.
// shouldn't be too difficult to generate
// run re-executes `mount` with each arg prefixed `subreq.Args += api/id.String+arg`
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
		cmds.CLI: mountFormatConsole,
	},
	Encoders: cmds.Encoders,
	Helptext: cmds.HelpText{
		Tagline:          MountTagline,
		ShortDescription: mountDescWhatAndWhere,
		LongDescription:  mountDescWhatAndWhere + "\nExample:\n" + mountDescExample,
	},
	Type: manager.Response{},
}

func mountPreRun(request *cmds.Request, env cmds.Environment) (err error) {
	// argument type checking
	if listArg, listArgProvided := request.Options[listOptionKwd]; listArgProvided {
		if value, isBool := listArg.(bool); !isBool {
			err = cmds.Errorf(cmds.ErrClient,
				"list parameter's argument (%#v) does not match expected type %T", listArg, value)
			return
		}
	}

	if len(request.Arguments) == 0 {
		request.Arguments = []string{
			"/fuse/ipfs",
			"/fuse/ipns",
		}
	}

	// TODO: this but properly
	if node, envErr := cmdenv.GetNode(env); envErr == nil {
		if !node.IsDaemon { // tell postrun to block forever
			request.Root.Extra = request.Root.Extra.SetValue("debug-foreground", true)
		}
	}

	return
}

// TODO: make sure our close semantics are correct for pre,post,run
// also make sure errors are properly wrapped in cmds.ErrX types (as late as possible)

func mountRun(request *cmds.Request, re cmds.ResponseEmitter, env cmds.Environment) (err error) {
	defer func() {
		if err != nil {
			err = errors.MaybeWrap(err, re.CloseWithError(err))
		}
	}()

	// TODO: some things are checked twice, we should do some databagging between phases
	// kind of like we're doing with the arg type checking
	// e.g. pre-run checks and stores node, instead of pre-run and run both doing it

	var (
		node *core.IpfsNode
		fsi  manager.Interface
		ctx  = request.Context
	)

	// parse Command's environment
	if node, err = cmdenv.GetNode(env); err != nil {
		err = cmds.Errorf(cmds.ErrNormal, "failed to get node instance from environment: %s", err)
		return
	}

	// use the node's interface if it has one
	// otherwise construct one for use in this run only
	if node.FileSystem != nil {
		fsi = node.FileSystem
	} else if fsi, err = NewNodeInterface(request.Context, node); err != nil {
		err = cmds.Errorf(cmds.ErrNormal, "failed to construct file system interface: %s", err)
		return
	}

	if listArg, _ := request.Options[listOptionKwd].(bool); listArg { // arg is type-checked in pre-run
		if err = errors.WaitFor(ctx,
			emitResponses(ctx, re,
				fsi.List(ctx)),
		); err != nil {
			err = cmds.Errorf(cmds.ErrNormal, "failed to list active instances: %s", err)
		}
		return
	}

	log.SetFlags(log.Lshortfile) // TODO: dbg lint

	requests, requestErrors := manager.ParseRequests(ctx, request.Arguments...)
	if err = errors.WaitFor(ctx, requestErrors,
		emitResponses(ctx, re,
			fsi.Bind(ctx, requests)),
	); err != nil {
		err = cmds.Errorf(cmds.ErrNormal, "failed to bind requests: %s", err)
	}
	return
}

// TODO: docs outdated; need to double check this and write in English.
// mainly responsible for setting the exit code in CLI requests
// an error should be returned from `Command.Run` if encountered
// especially an emitter error, so we put that at the front if encountered
func emitResponses(ctx context.Context, emitter cmds.ResponseEmitter, responses manager.Responses) errors.Stream {
	responseErrors := make(chan error)
	go func() {
		defer close(responseErrors)
		var emitErr error
		for response := range responses {
			/* TODO: try to remember why this was here; we probably wanted to wrap (in cmds) as late as possible
			if response.Error != nil {
				response.Error = cmds.Errorf(cmds.ErrNormal, response.Error.Error())
			}
			*/

			switch emitErr {
			case nil: // emitter has an observer (formatter, API client, etc.); try to emit to it
				if emitErr = emitter.Emit(response); emitErr != nil { // if the emitter encounters an error
					emitErr = cmds.Errorf(cmds.ErrNormal, "failed to emit response: %v: %s", response, emitErr)
					response.Error = errors.MaybeWrap(response.Error, emitErr) // make sure `errors` receives it
				}
			default: // emitter encountered an error during operation; don't try to emit to observers anymore
			}
			if response.Error != nil { // irrelevant of the emitter
				select { // we always try to send errors to the caller
				case responseErrors <- response.Error: // store errors for return value, regardless of emitter
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return responseErrors
}
