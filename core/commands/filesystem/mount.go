package fscmds

import (
	"fmt"
	"sync"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs/core/commands/cmdenv"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager"
)

var Mount = &cmds.Command{
	Helptext: cmds.HelpText{
		Tagline:          "Mounts IPFS to the filesystem.",
		ShortDescription: mountDescWhatAndWhere,
		LongDescription:  mountDescWhatAndWhere + "\nExample:\n" + mountDescExample,
	},
	Options: append(sharedOpts,
		cmds.BoolOption(mountListKwd, "l", mountListDesc),
	),
	Type: &Response{},
	//PostRun: cmds.PostRunMap{cmds.CLI: processResponse},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeEncoder(encodeText),
		cmds.JSON: cmds.MakeEncoder(encodeJSON),
	},
	Run: mountRun,
}

func mountRun(req *cmds.Request, re cmds.ResponseEmitter, env cmds.Environment) error {
	// `mount -l` requests
	if listArg, ok := req.Options[mountListKwd].(bool); ok && listArg {
		return listCommand(req, env, re)
	}

	// `mount` requests
	return bindCmd(req, env, re)
}

// TODO: parse API's from request, list specific ones if requested
// e.g.`ipfs mount -l --node=9p,fuse`
func listCommand(_ *cmds.Request, env cmds.Environment, re cmds.ResponseEmitter) error {
	node, err := cmdenv.GetNode(env)
	if err != nil {
		return fmt.Errorf("failed to get file instance node from request: %w", err)
	}

	responses := make(chan interface{}, 1) // NOTE: value must match `cmd.Command.Type`
	// ^ responses := make(chan Response, 1) // cmds lib needs it to be interface{}

	dispatcher := node.FileSystem

	// if the file instance dispatcher doesn't exist, we have nothing to list
	// so just relay that notice to the rpc client
	if dispatcher == nil {
		responses <- Response{Info: "None"}
		close(responses)
		return re.Emit(responses)
	}

	responses <- Response{Info: "host bindings:"}
	// for each API
	// process each response stream separately
	// merging into a unified channel
	go func() {
		for system := range dispatcher.List() {
			for hostResp := range system.FromHost {
				responses <- Response{
					Error: hostResp.Error,
					Request: manager.Request{
						Header: manager.Header{
							API: system.API,
							ID:  system.ID,
						},
						HostRequest: hostResp.Request,
					},
				}
			}
		}
		close(responses)
	}()

	return re.Emit(responses)
}

func bindCmd(req *cmds.Request, env cmds.Environment, re cmds.ResponseEmitter) error {
	requests, err := parseRequest(req, env)
	if err != nil {
		return err
	}

	if len(requests) == 0 {
		return nil
	}

	responses := make(chan interface{}, 1) // NOTE: value must match `cmd.Command.Type`
	// ^ responses := make(chan Response, 1) // cmds lib needs it to be interface{}
	responses <- Response{Info: "binding to host:"}

	node, err := cmdenv.GetNode(env)
	if err != nil {
		return fmt.Errorf("failed to get file instance node from request: %w", err)
	}

	dispatcher := node.FileSystem
	if dispatcher == nil {
		core, err := cmdenv.GetApi(env, req)
		if err != nil {
			return fmt.Errorf("failed to interface with the node: %w", err)
		}

		dispatcher, err = manager.NewDispatcher(node.Context(), core, node.FilesRoot)
		if err != nil {
			return fmt.Errorf("failed to construct file system interface: %w", err)
		}
		node.FileSystem = dispatcher
	}

	var wg sync.WaitGroup
	for resp := range dispatcher.Attach(requests...) {
		wg.Add(1)
		go func(resp manager.Response) { // for each host response channel
			for hostResp := range resp.FromHost { // merge host responses into the main response channel
				responses <- Response{ // (a copy of the response without the closer)
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

	go func() {
		wg.Wait()
		close(responses)
	}()

	return re.Emit(responses)
}
