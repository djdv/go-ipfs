package fscmds

import (
	"fmt"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs/core/commands/cmdenv"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager"
)

var Mount = &cmds.Command{
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
are set in the configuration file, but can be overridden by the options.
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
		cmds.BoolOption(mountListKwd, "l", mountListDesc),
	),
	Type: &Response{},
	//PostRun: cmds.PostRunMap{cmds.CLI: processResponse},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeEncoder(encodeText),
		cmds.JSON: cmds.MakeEncoder(encodeJson),
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

	responses <- Response{Info: "binding file systems to host:"}

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

	go func() {
		for host := range dispatcher.Attach(requests...) {
			for hostResp := range host.FromHost {
				responses <- Response{ // emit a copy without the closer
					Error: hostResp.Error,
					Request: manager.Request{
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

/*
func bindCmd(req *cmds.HostRequest, env cmds.Environment, re cmds.ResponseEmitter) error {
	// TODO: string emissions should all move to the emit handler
	// Emit{Err:infoError("Binding")
	// handleEmit(){ if infoError, print(err.Err()); return nil}
	re.Emit("Binding file systems...")

	node, err := cmdenv.GetNode(env)
	if err != nil {
		return fmt.Errorf("failed to get file instance node from request: %w", err)
	}

	requests, err := parseRequest(req, env)
	if err != nil {
		return err
	}

	if len(requests) == 0 {
		re.Emit("No binds requested")
		return nil
	}

	// NOTE:
	// the node's daemon-error-channel is set up by the daemon
	// the dispatcher is set up by us

	// TODO: torn down by `unmount` on the last instance
	dispatcher := node.FileSystem.Dispatcher
	if dispatcher == nil { // so instantiate it if it's not there
		coreAPI, err := cmdenv.GetApi(env, req)
		if err != nil {
			return fmt.Errorf("failed to node file instance with node: %w", err)
		}

		var managerOpts []_interface.Option

		if node.FilesRoot != nil { // TODO: should we just always do this?
			managerOpts = append(managerOpts, _interface.WithFilesAPIRoot(node.FilesRoot))
		}
		// TODO: option like `IsOffline` for the dispatcher; set's IPNS publisher to node, etc.
		// set when !IsDaemon

		dispatcher, err = fsn.NewDispatcher(node.Context(), coreAPI, managerOpts...)
		if err != nil {
			return err
		}
		node.FileSystem.Dispatcher = dispatcher
	}

	go func() {
		for res := range dispatcher.Attach(requests...) {
			re.Emit(res)
		}
	}()

	if !node.IsDaemon {
		// if this command isn't running on a daemon
		// block until the node's context is canceled
		// the binds will be active for as long as the node exists
		// and closed via the node's own shutdown mechanism
		<-node.Context().Done()
	}
	return
}
*/
