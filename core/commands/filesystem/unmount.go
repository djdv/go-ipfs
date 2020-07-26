package fscmds

import (
	"errors"
	"sync"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs/core/commands/cmdenv"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager"
)

var Unmount = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline: "Removes IPFS mountpoints from the filesystem.",
		ShortDescription: `
		TODO: replace this text :^)
`,
		LongDescription: `
		TODO: replace this text :^)
`,
	},
	Options: append(sharedOpts,
		cmds.BoolOption(unmountAllKwd, "a", unmountAllDesc)),
	Type: &Response{},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeEncoder(encodeText),
		cmds.JSON: cmds.MakeEncoder(encodeJson),
	},
	Run: unmountRun,
}

func unmountRun(req *cmds.Request, re cmds.ResponseEmitter, env cmds.Environment) (err error) {
	node, err := cmdenv.GetNode(env)
	if err != nil {
		return err
	}

	responses := make(chan interface{}, 1) // NOTE: value must match `cmd.Command.Type`
	// ^ responses := make(chan Response, 1) // cmds lib needs it to be interface{}
	responses <- Response{Info: "detaching from host:"}

	dispatcher := node.FileSystem

	// if the file instance dispatcher doesn't exist, we have nothing to detach
	if dispatcher == nil {
		responses <- Response{Error: errors.New("no file system manager initialized")}
		close(responses)
		return re.Emit(responses)
	}

	if detachArg, ok := req.Options[unmountAllKwd].(bool); ok && detachArg {
		go func() {
			for resp := range CloseFileSystem(dispatcher) {
				responses <- resp
			}
			node.FileSystem = nil // remove self from the node, don't remain in empty state
			close(responses)
		}()
		return re.Emit(responses)
	}

	requests, err := parseRequest(req, env)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup

	// TODO: look here again; can we merge the response closer or does it have to be independent?
	// *⬇
	for resp := range dispatcher.Detach(requests...) {
		wg.Add(1)                        // for each host response channel
		go func(resp manager.Response) { // for each host response channel
			for hostResp := range resp.FromHost { // merge host responses into the main response channel
				responses <- Response{
					Error: hostResp.Error,
					Request: manager.Request{
						Header:      resp.Header,
						HostRequest: hostResp.Request,
					},
				}
			}
			wg.Done()
		}(resp)
	}

	go func() { // *⬆
		wg.Wait() // wait for all hosts to respond before closing responses

		// HACK: needs formalization; List needs ctx cancel too
		// we want the dispatcher to remove itself on final close
		// (pass node to constructor, add detach wrapper to apiMux)
		empty := true
		for range dispatcher.List() {
			empty = false
			// and continue to drain the channel because no cancel
		}

		if empty { // XXX: no sync
			node.FileSystem = nil // remove self from the node, don't remain in empty state
		}

		close(responses)
	}()

	return re.Emit(responses)
}
