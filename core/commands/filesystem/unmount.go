package fscmds

import (
	"errors"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs/core/commands/cmdenv"
	fsm "github.com/ipfs/go-ipfs/core/commands/filesystem/manager"
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
	Options: append(cmdSharedOpts,
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
				// FIXME: needs processing; this is sending the wrong type
				responses <- resp
			}
			close(responses)
		}()
		return re.Emit(responses)
	}

	requests, err := parseRequest(req, env)
	if err != nil {
		return err
	}

	go func() {
		for host := range dispatcher.Detach(requests...) {
			for hostResp := range host.FromHost {
				responses <- Response{ // emit a copy without the closer
					Error: hostResp.Error,
					Request: fsm.Request{
						Header:      host.Header,
						HostRequest: hostResp.Request,
					},
				}
			}
		}
		close(responses)
	}()

	return re.Emit(responses)
}
