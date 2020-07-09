package fscmds

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager"

	"github.com/ipfs/go-ipfs/filesystem"

	cmds "github.com/ipfs/go-ipfs-cmds"
)

var (
	ErrInvalidSystem = errors.New("unknown system requested")
	ErrInvalidAPI    = errors.New("unknown API requested")

	cmdSharedOpts = []cmds.Option{
		cmds.StringOption(aPIKwd, aPIDesc),
		cmds.StringOption(subsystemKwd, subsystemDesc),
		cmds.StringOption(TargetKwd, targetDesc),
	}

	DaemonOpts = []cmds.Option{
		cmds.BoolOption(DaemonCmdPrefix, daemonBindDesc),
		cmds.StringOption(daemonFSAPIKwd, daemonDescInfo+aPIDesc),
		cmds.StringOption(daemonSubsystemKwd, daemonDescInfo+subsystemDesc),
		cmds.StringOption(daemonTargetKwd, daemonDescInfo+targetDesc),
	}
)

const (

	// Commands that forward arguments to our Command
	// should use prefixed parameters, and translate their requests
	// forwarding them to the corresponding command you wish to target

	DaemonCmdPrefix  = "mount"               // `ipfs daemon --mount ...` => `ipfs mount ...`
	daemonCmdsPrefix = DaemonCmdPrefix + "-" // `... --mount-mountParam=paramArg` => `... --mountParam=paramArg`

	daemonBindDesc = "Binds IPFS APIs to the host system" // the description for the prefix itself
	// ^ TODO: this should pull
	// i.e. all prefixed commands should use the same description
	// as `mount` itself does

	mountListKwd  = "list"
	mountListDesc = "List mounted instances."

	aPIKwd         = "system"
	aPIDesc        = "Selects which file system API to use, defaults to config file value or " + defaultAPIOption + " (on this machine)"
	daemonFSAPIKwd = daemonCmdsPrefix + aPIKwd

	subsystemKwd       = "subsystem"
	daemonSubsystemKwd = daemonCmdsPrefix + subsystemKwd
	subsystemDesc      = "A comma separated list of system APIs to operate on. Defaults to config setting or platform appropriate value"

	TargetKwd       = "target"
	daemonTargetKwd = daemonCmdsPrefix + TargetKwd
	targetDesc      = "A comma separated list of path to use. Defaults to config setting or platform appropriate value."

	unmountAllKwd  = "all"
	unmountAllDesc = "Unmount all instances."

	// all daemon descriptions should include this message
	// in addition to the parameters normal description
	daemonDescInfo = "(if using --mount) "

	// TODO: templateRoot = (*nix) `/` || (NT) ${CurDrive}:\ || (any others)...
	templateHome = "IPFS_HOME"
)

func typeCastSystemArg(in string) (filesystem.ID, error) {
	systemTarget, ok := map[string]filesystem.ID{
		strings.ToLower(filesystem.IPFS.String()):  filesystem.IPFS,
		strings.ToLower(filesystem.IPNS.String()):  filesystem.IPNS,
		strings.ToLower(filesystem.Files.String()): filesystem.Files,
		strings.ToLower(filesystem.PinFS.String()): filesystem.PinFS,
		strings.ToLower(filesystem.KeyFS.String()): filesystem.KeyFS,
	}[strings.ToLower(in)]
	if !ok {
		var none filesystem.ID
		return none, fmt.Errorf("%w:%s", ErrInvalidSystem, in)
	}
	return systemTarget, nil
}

func typeCastAPIArg(in string) (manager.API, error) {
	apiTarget, ok := map[string]manager.API{
		strings.ToLower(manager.Plan9Protocol.String()): manager.Plan9Protocol,
		strings.ToLower(manager.Fuse.String()):          manager.Fuse,
	}[strings.ToLower(in)]
	if !ok {
		var none manager.API
		return none, fmt.Errorf("%w:%s", ErrInvalidAPI, in)
	}
	return apiTarget, nil
}
