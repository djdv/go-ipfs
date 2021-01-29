package fscmds

import (
	"fmt"
	"strings"
	"sync"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs/core/commands/cmdenv"
	"github.com/ipfs/go-ipfs/filesystem/manager"
)

// TODO: copy paste hacks
const (
	UnmountParameter           = "unmount"
	UnmountArgumentDescription = "Multiaddr style targets to bind with host. (/fuse/ipfs/path/ipfs)"
	unmountAllOptionKwd        = "all"
)

var Unmount = &cmds.Command{
	Options: []cmds.Option{
		cmds.BoolOption(unmountAllOptionKwd, "a", "close all active instances"),
	},
	Arguments: []cmds.Argument{
		cmds.StringArg("targets", false, true, MountArgumentDescription),
	},
	//PreRun: unmountPreRun, // TODO: make sure len(targets) == 0 if -a provided, otherwise error
	Run: unmountRun,
	PostRun: cmds.PostRunMap{
		cmds.CLI: unmountPostRunCLI,
	},
	Encoders: cmds.Encoders,
	//Helptext: cmds.HelpText{
	//Tagline:          MountTagline,
	//ShortDescription: mountDescWhatAndWhere,
	//LongDescription:  mountDescWhatAndWhere + "\nExample:\n" + mountDescExample,
	//},
	Type: manager.Response{},
}

// TODO: whole file is quick hacks to get working

func unmountRun(request *cmds.Request, re cmds.ResponseEmitter, env cmds.Environment) error {
	node, err := cmdenv.GetNode(env)
	if err != nil {
		return err
	}

	var closeAll bool // `--all`
	if allFlag, provided := request.Options[unmountAllOptionKwd]; provided {
		if flag, isBool := allFlag.(bool); isBool {
			closeAll = flag
		} else {
			return paramError(unmountAllOptionKwd, allFlag, closeAll)
		}
	}

	var match func(instance manager.Response) bool
	if closeAll {
		match = func(manager.Response) bool { return true }
	} else {
		match = func(instance manager.Response) bool {
			for _, instanceTarget := range request.Arguments {
				if instance.String() == instanceTarget {
					return true
				}
			}
			return false
		}
	}

	var wg sync.WaitGroup
	for instance := range node.FileSystem.List(request.Context) {
		wg.Add(1)
		go func(instance manager.Response) {
			defer wg.Done()
			if match(instance) {
				instance.Error = instance.Close()
				err = re.Emit(instance) // TODO emitter's error
			}
		}(instance)
	}
	wg.Wait()
	return err
}

func unmountPostRunCLI(response cmds.Response, emitter cmds.ResponseEmitter) (err error) {
	encType := cmds.GetEncoding(response.Request(), "")
	consoleOut, emit := printXorRelay(encType, emitter)
	decoWrite := func(s string) (err error) { _, err = consoleOut.Write([]byte(s)); return }

	// TODO: different message for closeAll
	decoWrite(fmt.Sprintf("closing: %s\n",
		strings.Join(response.Request().Arguments, ", ")))

	ctx := response.Request().Context
	responses := cmdsResponseToManagerResponses(ctx, response)
	for fsResponse := range drawResponses(ctx, consoleOut, responses) {
		err = emit(fsResponse) // TODO: accumulate emitter errors
	}

	return
}
