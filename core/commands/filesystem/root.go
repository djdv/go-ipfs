package fscmds

import (
	"context"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs/filesystem/manager"
	"github.com/ipfs/go-ipfs/filesystem/manager/errors"
)

const (
	serviceTarget = "filesystem.service"

	//ipcDaemonOptionKwd         = "daemon"
	//ipcDaemonOptionDescription = "TODO: daemon help text; it waits in the background and maintains connections to the IPFS node."

	rootServiceOptionKwd         = "api"
	rootServiceOptionDescription = "File system service multiaddr to use."

	rootIPFSOptionKwd         = "ipfs"
	rootIPFSOptionDescription = "IPFS API multiaddr to use."
)

var ClientRoot = &cmds.Command{
	Options: []cmds.Option{
		cmds.StringOption(rootServiceOptionKwd, rootServiceOptionDescription),
		cmds.StringOption(rootIPFSOptionKwd, rootIPFSOptionDescription),

		// TODO: consider file-relevant cmds pkg options (symlinks, hidden attribute, etc.)
		// for dealing with fs/mtab-like input file
		cmds.OptionEncodingType,
		cmds.OptionTimeout,
		cmds.OptionStreamChannels, // TODO: what does this imply? Streaming the cmds RPC values?
		// TODO: PR? why are the flags defined but the options are not? (cmds functions check for them internally)
		cmds.BoolOption(cmds.OptLongHelp, "Show the full command help text."),
		cmds.BoolOption(cmds.OptShortHelp, "Show a short version of the command help text."),
	},
	Subcommands: map[string]*cmds.Command{
		mountParameter:   Mount,
		unmountParameter: Unmount,
		listParameter:    List,
	},
}

var FullRoot = ClientRoot

func init() { FullRoot.Subcommands[serviceParameter] = Service }

func emitResponses(ctx context.Context, emit cmdsEmitFunc, requestErrors errors.Stream, responses manager.Responses) (allErrs []error) {
	var emitErr error
	for responses != nil || requestErrors != nil {
		select {
		case response, ok := <-responses:
			if !ok {
				responses = nil
				continue
			}
			if emitErr = emit(response); emitErr != nil {
				allErrs = append(allErrs, emitErr)            // emitter encountered a fault
				emit = func(interface{}) error { return nil } // stop emitting values to its observer
			}
			if response.Error != nil {
				allErrs = append(allErrs, response.Error)
			}

		case err, ok := <-requestErrors:
			if !ok {
				requestErrors = nil
				continue
			}
			allErrs = append(allErrs, err)

		case <-ctx.Done():
			return
		}
	}
	return
}
