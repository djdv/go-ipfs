package fscmds

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs-cmds/cli"
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

// TODO: english pass
// construct subcommand groups from supported API/ID pairs
// e.g. make these invocations equal
// 1) `ipfs mount /fuse/ipfs/path/mountpoint /fuse/ipfs/path/mountpoint2 ...
// 2) `ipfs mount fuse /ipfs/path/mountpoint /ipfs/path/mountpoint2 ...
// 3) `ipfs mount fuse ipfs /path/mountpoint /path/mountpoint2 ...
// allow things like `ipfs mount fuse -l` to list all fuse instances only, etc.
// shouldn't be too difficult to generate
// run re-executes `mount` with each arg prefixed `subreq.Args += api/id.String+arg`
func init() { registerSubcommands(Mount); return }

func registerSubcommands(parent *cmds.Command) {
	template := &cmds.Command{
		Arguments: []cmds.Argument{
			cmds.StringArg("targets", false, true, MountArgumentDescription),
		},
		Run:      parent.Run,
		PostRun:  parent.PostRun,
		Encoders: parent.Encoders,
	}

	subcommands := make(map[string]*cmds.Command)

	for _, api := range []filesystem.API{
		filesystem.Fuse,
		filesystem.Plan9Protocol,
	} {
		hostName := api.String()
		subsystems := make(map[string]*cmds.Command)
		for _, id := range []filesystem.ID{
			filesystem.IPFS,
			filesystem.IPNS,
		} {
			nodeName := id.String()
			com := new(cmds.Command)
			*com = *template
			// TODO: filtered --list + helptext (use some fmt tmpl)
			com.PreRun = func(request *cmds.Request, env cmds.Environment) (err error) {
				foregroundHacks(request, env)
				for i, arg := range request.Arguments {
					// TODO: special case per host-API (hard coded `/path/` for now)
					// 9P should probably be raw value to accept path or tcp or both
					request.Arguments[i] = fmt.Sprintf("/%s/%s/path/%s", hostName, nodeName, strings.TrimPrefix(arg, "/"))
				}
				return nil
			}
			subsystems[nodeName] = com
		}

		com := new(cmds.Command)
		*com = *template
		com.PreRun = func(request *cmds.Request, env cmds.Environment) (err error) {
			foregroundHacks(request, env)
			for i, arg := range request.Arguments {
				request.Arguments[i] = fmt.Sprintf("/%s/%s", hostName, strings.TrimPrefix(arg, "/"))
			}
			return nil
		}
		com.Subcommands = subsystems
		subcommands[hostName] = com
	}

	parent.Subcommands = subcommands
	return
}

// TODO: this but properly
func foregroundHacks(request *cmds.Request, env cmds.Environment) {
	if node, envErr := cmdenv.GetNode(env); envErr == nil {
		if !node.IsDaemon { // tell postrun to block
			request.Root.Extra = request.Root.Extra.SetValue("debug-foreground", true)
			log.Println("setting foreground")
		} else {
			log.Println("not setting foreground:", node.IsDaemon)
		}
	}
}

func mountPreRun(request *cmds.Request, env cmds.Environment) (err error) {
	if len(request.Arguments) == 0 {
		request.Arguments = []string{
			"/fuse/ipfs",
			"/fuse/ipns",
		}
	}
	foregroundHacks(request, env)

	return
}

func mountRun(request *cmds.Request, re cmds.ResponseEmitter, env cmds.Environment) (err error) {
	log.SetFlags(log.Lshortfile) // TODO: dbg lint
	const errParameterTypeFmt = "argument (%v) is type: %T, expecting type: %T"

	var doList bool // `--list`
	if listFlag, provided := request.Options[listOptionKwd]; provided {
		if flag, ok := listFlag.(bool); ok {
			doList = flag
		}
		err = cmds.Errorf(cmds.ErrClient,
			"list "+errParameterTypeFmt,
			listFlag, listFlag, doList)
		return
	}

	defer func() {
		if err != nil {
			err = cmds.Errorf(cmds.ErrNormal, "%s", err)
			if emitterErr := re.CloseWithError(err); emitterErr != nil {
				err = cmds.Errorf(cmds.ErrNormal, "%s - additionally an emitter error was encountered: %s", err, emitterErr)
			}
		}
	}()

	var node *core.IpfsNode
	if node, err = cmdenv.GetNode(env); err != nil {
		err = fmt.Errorf("failed to get node instance from environment: %w", err)
		return
	}

	var fsi manager.Interface
	if node.FileSystem != nil { // use the node's interface if it has one
		fsi = node.FileSystem // otherwise construct one for this run only
	} else if fsi, err = NewNodeInterface(request.Context, node); err != nil {
		err = fmt.Errorf("failed to construct file system interface: %w", err)
		return
	}

	ctx, cancel := context.WithCancel(request.Context)
	defer cancel()

	if doList {
		err = errors.WaitFor(ctx,
			responsesToEmitter(ctx, re,
				fsi.List(ctx)))
		return
	}

	requests, requestErrors := manager.ParseRequests(ctx, request.Arguments...)
	err = errors.WaitForAny(ctx, requestErrors,
		responsesToEmitter(ctx, re,
			fsi.Bind(ctx, requests)))

	return
}

//TODO: English
// responsesToEmitter relays manager responses to an emitter,
// returning a combined stream of errors supplied from the response values and the emitter itself.
func responsesToEmitter(ctx context.Context, emitter cmds.ResponseEmitter, responses manager.Responses) errors.Stream {
	combinedErrors := make(chan error)
	go func() {
		defer close(combinedErrors)
		var emitErr error
		for response := range responses {
			switch emitErr {
			case nil: // emitter has an observer (formatter, API client, etc.); try to emit to it
				if emitErr = emitter.Emit(response); emitErr != nil { // if the emitter encounters an error
					response.Error = maybeWrap(response.Error, emitErr) // make sure `combinedErrors` receives it
				}
			default: // emitter encountered an error during operation; don't try to emit to observers anymore
			}
			if response.Error != nil { // always try to send to the caller
				select { // (irrelevant of the emitter)
				case combinedErrors <- response.Error:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return combinedErrors
}

// TODO: hardcoded async stdout printing ruins our output :^(
// the logger gets a better error than the caller too...
// https://github.com/bazil/fuse/blob/371fbbdaa8987b715bdd21d6adc4c9b20155f748/mount_linux.go#L98-L99
// TODO: needs to be broken up more, clear split between foreground and background requests, as well as formatted vs encoded
// TODO: support `--enc`; if request encoding type is not "text", don't print extra messages or render the table
// just write responses directly to stdout
//
// formats responses for CLI/terminal displays
func mountPostRunCLI(res cmds.Response, re cmds.ResponseEmitter) (err error) {
	const errParameterTypeFmt = "argument (%v) is type: %T, expecting type: %T"
	// NOTE: We expect CLI encoding to be text.
	// If it is, we'll format output for a terminal.
	// Otherwise, we'll pass values through to the emitter.
	// (which is likely: encoderMap(value) => stdout)
	encType := cmds.GetEncoding(res.Request(), "")

	var isList bool // `--list`
	if listFlag, provided := res.Request().Options[listOptionKwd]; provided {
		if flag, ok := listFlag.(bool); ok {
			isList = flag
		}
		err = cmds.Errorf(cmds.ErrClient,
			"list "+errParameterTypeFmt,
			listFlag, listFlag, isList)
		return
	}

	var console = re.(cli.ResponseEmitter)
	if foregroundArg, ok := res.Request().Root.Extra.GetValue("debug-foreground"); ok {
		if isForeground, _ := foregroundArg.(bool); isForeground {
			log.Println("foreground baybee:", isForeground)
			foregroundCtx, foregroundCancel := context.WithCancel(res.Request().Context)
			foregroundInstances := make([]manager.Response, 0, len(res.Request().Arguments))
			// if this request spawned the node,
			// block until we receive an error or an interrupt
			defer func() {
				log.Println("foreground run returned: ", err)
				if err == nil {
					if encType == cmds.Text {
						if _, err = console.Stdout().Write([]byte("Waiting in foreground, send interrupt to cancel\n")); err != nil {
							err = fmt.Errorf("emitter encountered an error, exiting early: %w", err)
							return
						}
					}
				} else {
					foregroundCancel()
				}

				<-foregroundCtx.Done()
				log.Println("foreground closing...")

				// TODO: currently this is empty and the process zombies mounts in mtab
				// we need to intercept between bind

				// TODO: bail on write errors or something
				for _, instance := range foregroundInstances {
					console.Stdout().Write([]byte((fmt.Sprintf("closing 📖 %v ...\n", instance))))
					switch err := instance.Close(); err {
					case nil:
						console.Stdout().Write([]byte((fmt.Sprintf("closed 📗 %v\n", instance))))
					default:
						console.Stdout().Write([]byte((fmt.Sprintf("failed to detach 📕 %v from host: %v\n", instance, err))))
					}
				}
			}()
		}
	}

	defer func() {
		if err != nil {
			err = cmds.Errorf(cmds.ErrNormal, "%s", err)
			if emitterErr := re.CloseWithError(err); emitterErr != nil {
				err = fmt.Errorf("%s - additionally an emitter error was encountered: %w", err, emitterErr)
			}
		}
	}()

	if encType != cmds.Text {
		err = mountPostRunRaw(res, re)
		return
	}

	ctx := res.Request().Context
	if isList {
		err = drawList(ctx, console, res)
	} else {
		err = drawBind(ctx, console, res)
	}

	return
}

func drawBind(ctx context.Context, console cli.ResponseEmitter, response cmds.Response) error {
	stringArgs := response.Request().Arguments
	if len(stringArgs) != 0 {
		msg := fmt.Sprintf("Attempting to bind to host system: %s...\n", strings.Join(stringArgs, ", "))
		if _, err := console.Stdout().Write([]byte(msg)); err != nil {
			return err
		}
	}

	// hax do better
	responses := consoleResponseToManager(ctx, console, response)
	relay := make(chan manager.Response, len(responses))
	var lastError error
	//

	go func() {
		for resp := range responses {
			if resp.Error != nil {
				lastError = resp.Error // TODO: accumulate
			}
			relay <- resp
		}
		close(relay)
	}()

	// TODO [current]: relay emitter stream to drawResponse with conditions between
	// if errors are encountered, accumulate them and return them, but keep drawing in-between

	for range drawResponses(ctx, console, relay) {
		// this channel draining does the drawing
	}

	return lastError
}

func drawList(ctx context.Context, console cli.ResponseEmitter, response cmds.Response) error {
	var gotResponse bool

	responses := consoleResponseToManager(ctx, console, response)
	relay := make(chan manager.Response, len(responses))

	go func() {
		for response := range responses {
			gotResponse = true
			relay <- response
		}
		close(relay)
	}()

	for range drawResponses(ctx, console, relay) {
		// this channel draining does the drawing
	}

	var err error
	if !gotResponse {
		_, err = console.Stdout().Write([]byte("No active instances\n"))
	}
	return err
}

// just passes responses to the emitter as-is
// TODO: English pass
// [magic; cmds-lib]
// if the encoding type isn't text,
// we don't insert any terminal-specific data in our output.
// Instead we just relay values as-is,
// allowing the emitter to handle them however it wants to.
// (in this context, it's most likely going to do `cmd.EncoderMap[...](value) => stdout`)
// This allows things like `ipfs mount --enc=json | jq ...` to work.
func mountPostRunRaw(res cmds.Response, re cmds.ResponseEmitter) (err error) {
	for {
		var v interface{}
		if v, err = res.Next(); err != nil {
			if err == io.EOF { // no more responses
				err = nil
				break
			}
			return // some kind of emitter error was encountered
		}
		if err = re.Emit(v); err != nil {
			return
		}
	}
	return
}

//TODO: English
// emitterToResponses relays emitter responses as manager.Responses,
// if the emitter encounters an error, the response stream is closed.
func consoleResponseToManager(ctx context.Context, console cli.ResponseEmitter, res cmds.Response) manager.Responses {
	responses := make(chan manager.Response)
	go func() {
		defer close(responses)
		for {
			untypedResponse, err := res.Next()
			if err != nil {
				return
				/* TODO: maybe return these in a separate error channel
				if err == io.EOF { // no more responses
					break
				}
				select {
				case responses <- manager.Response{Error: err}:
				case <-ctx.Done():
				}
				return
				*/
			}
			// TODO: I don't know if this is intentional or not
			// Emit will return a pointer to us if the command was executed remotely
			// and a non-pointer if local; regardless of the `Command.Type`'s type and what was actually passed to `Emit`
			// maybe a cmds-lib bug?
			var response manager.Response
			switch v := untypedResponse.(type) {
			case manager.Response:
				response = v
			case *manager.Response:
				response = *v
			default: // TODO: better solution, server/run can trigger a client panic if they emit bad types
				log.Println("bout to panic:", untypedResponse)
				panic(fmt.Errorf("formatter received unexpected type+value from emitter: %#v", untypedResponse))
			}
			select {
			case responses <- response:
			case <-ctx.Done():
			}
		}
	}()
	return responses
}
