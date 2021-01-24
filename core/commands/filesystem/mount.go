package fscmds

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"time"

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

// TODO: English pass; try to break appart code too, this is gross
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
				if err = maybeSetPostRunFunc(request, env); err != nil {
					return
				}
				for i, arg := range request.Arguments {
					// TODO: special case per host-API (hard coded `/path/` for now)
					// 9P should probably be raw value to accept path or tcp or both
					request.Arguments[i] = fmt.Sprintf("/%s/%s/path/%s", hostName, nodeName, strings.TrimPrefix(arg, "/"))
				}
				return
			}
			subsystems[nodeName] = com
		}

		com := new(cmds.Command)
		*com = *template
		com.PreRun = func(request *cmds.Request, env cmds.Environment) (err error) {
			if err = maybeSetPostRunFunc(request, env); err != nil {
				return
			}
			for i, arg := range request.Arguments {
				request.Arguments[i] = fmt.Sprintf("/%s/%s", hostName, strings.TrimPrefix(arg, "/"))
			}
			return
		}
		com.Subcommands = subsystems
		subcommands[hostName] = com
	}

	parent.Subcommands = subcommands
	return
}

// TODO: emitter should probably be typecast inside of postrun, accepting the general cmds.RE instead
type postRunFunc func(context.Context, cmds.Response, cmds.ResponseEmitter) errors.Stream

const postRunFuncKey = "👻" // arbitrary index value that's smaller than its description

// try to come up with exposable compliment; e.g. something that stacks {daemonPart;fmtCloseAll}, {mountPart;fmtCloseAll}
func waitAndTeardown(fsi manager.Interface) postRunFunc {
	return func(ctx context.Context, res cmds.Response, re cmds.ResponseEmitter) errors.Stream {
		var (
			errs               = make(chan error, 1)
			out                = ioutil.Discard
			emit               = re.Emit
			console, isConsole = re.(cli.ResponseEmitter)
			encType            = cmds.GetEncoding(res.Request(), "")
			decorate           = isConsole && encType == cmds.Text
			decoWrite          = func(s string) (err error) { _, err = out.Write([]byte(s)); return }
		)
		defer close(errs)

		if decorate {
			out = console.Stdout()
			emit = func(interface{}) error { return nil }
			if err := decoWrite("Waiting in foreground, send interrupt to cancel\n"); err != nil {
				errs <- fmt.Errorf("emitter encountered an error, exiting early: %w", err)
				return errs
			}
		}

		<-ctx.Done()

		go func() {
			// TODO: arbitrary time duration
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			var err error
			for instance := range fsi.List(ctx) { // TODO: timeout ctx
				if err = decoWrite(fmt.Sprintf("closing %v ...\n", instance)); err != nil {
					errs <- err
				}

				err = instance.Close() // NOTE: we're (re-)using the fs output value
				instance.Error = err   // as/for the emitter's input value

				switch err {
				case nil:
					if err = decoWrite(fmt.Sprintf("closed %v\n", instance)); err != nil {
						errs <- err
					}
				default:
					errs <- err
					if err = decoWrite(fmt.Sprintf("failed to detach 📕 %v from host: %v\n", instance, err)); err != nil {
						errs <- err
					}
				}

				// [magic] cmds-lib
				// if the emitter passed to us isn't a text-console
				// the emitter will likely use the command's encoder map to encode the value
				// to whatever non-plaintext format it requested (e.g. JSON, XML, et al.)
				if err = emit(instance); err != nil {
					errs <- err
				}
			}
			return
		}()
		return errs
	}
}

// special handling for things like local-only requests
func maybeSetPostRunFunc(request *cmds.Request, env cmds.Environment) (err error) {
	var node *core.IpfsNode
	if node, err = cmdenv.GetNode(env); err != nil {
		return
	}
	// `ipfs daemon` will set up the fsi for us, and close it when it's done.
	// But if the daemon isn't running, it won't exist on the node.
	// So we spawn one that will close when this request is done (after PostRun returns).
	if !node.IsDaemon && node.FileSystem == nil {
		var fsi manager.Interface
		if fsi, err = NewNodeInterface(request.Context, node); err != nil {
			err = fmt.Errorf("failed to construct file system interface: %w", err)
			return
		}

		var postFunc postRunFunc = func(ctx context.Context, res cmds.Response, re cmds.ResponseEmitter) errors.Stream {
			node.FileSystem = nil
			return waitAndTeardown(fsi)(ctx, res, re)
		}

		//request.Root.Extra = request.Root.Extra.SetValue(postRunFuncKey, waitAndTeardown(fsi))
		request.Root.Extra = request.Root.Extra.SetValue(postRunFuncKey, postFunc)
		node.FileSystem = fsi
	}
	return
}

func mountPreRun(request *cmds.Request, env cmds.Environment) (err error) {
	if len(request.Arguments) == 0 {
		request.Arguments = []string{
			"/fuse/ipfs",
			"/fuse/ipns",
		}
	}

	//TODO [current]: init fsi here, set in extra
	// if it exist, and not `isList`, call close(responses) from postrun.extra.fsi
	// more succinct, make sure fsi.Close is called (in postrun) if spawned (in prerun)
	// ^close must be a closure, not a method, but yes

	err = maybeSetPostRunFunc(request, env)
	return
}

func mountRun(request *cmds.Request, emitter cmds.ResponseEmitter, env cmds.Environment) (err error) {
	log.SetFlags(log.Lshortfile) // TODO: dbg lint
	const errParameterTypeFmt = "argument (%v) is type: %T, expecting type: %T"

	var doList bool // `--list`
	if listFlag, provided := request.Options[listOptionKwd]; provided {
		if flag, isBool := listFlag.(bool); isBool {
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
			if emitterErr := emitter.CloseWithError(err); emitterErr != nil {
				err = cmds.Errorf(cmds.ErrNormal, "%s - additionally an emitter error was encountered: %s", err, emitterErr)
			}
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

	if doList {
		err = errors.WaitFor(ctx,
			responsesToEmitter(ctx, emitter,
				fsi.List(ctx)))
		return
	}

	requests, requestErrors := manager.ParseRequests(ctx, request.Arguments...)
	err = errors.WaitForAny(ctx, requestErrors,
		responsesToEmitter(ctx, emitter,
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
func mountPostRunCLI(response cmds.Response, emitter cmds.ResponseEmitter) (err error) {
	const errParameterTypeFmt = "argument (%v) is type: %T, expecting type: %T"
	// NOTE: We expect CLI encoding to be text.
	// If it is, we'll format output for a terminal.
	// Otherwise, we'll pass values through to the emitter.
	// (which is likely: encoderMap(value) => stdout)

	var isList bool // `--list`
	if listFlag, provided := response.Request().Options[listOptionKwd]; provided {
		if flag, ok := listFlag.(bool); ok {
			isList = flag
		}
		err = cmds.Errorf(cmds.ErrClient,
			"list "+errParameterTypeFmt,
			listFlag, listFlag, isList)
		return
	}

	defer func() {
		if err != nil {
			err = cmds.Errorf(cmds.ErrNormal, "%s", err)
			if emitterErr := emitter.CloseWithError(err); emitterErr != nil {
				err = fmt.Errorf("%s - additionally an emitter error was encountered: %w", err, emitterErr)
			}
			return
		}
		postRunFuncArg, provided := response.Request().Root.Extra.GetValue(postRunFuncKey)
		if !provided {
			return
		}

		postFunc, isFunc := postRunFuncArg.(postRunFunc)
		if !isFunc { // TODO: sloppy copy paste
			const errParameterTypeFmt = "argument (%v) is type: %T, expecting type: %T"
			err = cmds.Errorf(cmds.ErrClient,
				"postRun "+errParameterTypeFmt,
				postRunFuncArg, postRunFuncArg, postFunc)
			return
		}
		postFunc(response.Request().Context, response, emitter)
	}()

	// FIXME: names, should be doList, doBind, where drawing is implicit depending on re's type
	// NOT dependant on stdout being available
	// e.g. pass in out and emit,
	ctx := response.Request().Context
	if isList {
		err = drawList(ctx, emitter, response)
	} else {
		err = drawBind(ctx, emitter, response)
	}

	return
}

func drawBind(ctx context.Context, emitter cmds.ResponseEmitter, response cmds.Response) error {
	console, isConsole := emitter.(cli.ResponseEmitter)
	if !isConsole {
		panic("output to this endpoint is not supported yet")
	}

	stringArgs := response.Request().Arguments
	if len(stringArgs) != 0 {
		msg := fmt.Sprintf("Attempting to bind to host system: %s...\n", strings.Join(stringArgs, ", "))
		if _, err := console.Stdout().Write([]byte(msg)); err != nil {
			return err
		}
	}

	// hax do better
	responses := cmdsResponseToManagerResponse(ctx, response)
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

func drawList(ctx context.Context, emitter cmds.ResponseEmitter, response cmds.Response) error {
	var gotResponse bool
	console, isConsole := emitter.(cli.ResponseEmitter)
	if !isConsole {
		panic("output to this endpoint is not supported yet")
	}

	responses := cmdsResponseToManagerResponse(ctx, response)
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

//TODO: English
// cmdsResponseToManagerResponse relays emitter responses as manager.Responses,
// if the emitter encounters an error, the response stream is closed.
func cmdsResponseToManagerResponse(ctx context.Context, response cmds.Response) manager.Responses {
	responses := make(chan manager.Response)
	go func() {
		defer close(responses)
		for {
			untypedResponse, err := response.Next()
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
