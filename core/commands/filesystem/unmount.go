package fscmds

import (
	"fmt"
	"strings"
	"sync"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs/core/commands/cmdenv"
	"github.com/ipfs/go-ipfs/filesystem/manager"
)

const (
	UnmountParameter            = "unmount"
	UnmountArgumentDescription  = "Multiaddr style targets to detach from host. " + mountTargetExamples
	unmountAllOptionKwd         = "all"
	unmountAllOptionDescription = "close all active instances (exclusive: do not provide arguments with this flag)"
)

var Unmount = &cmds.Command{
	Options: []cmds.Option{
		cmds.BoolOption(unmountAllOptionKwd, "a", unmountAllOptionDescription),
	},
	Arguments: []cmds.Argument{
		cmds.StringArg(mountStringArgument, false, true, UnmountArgumentDescription),
	},
	PreRun: unmountPreRun,
	Run:    unmountRun,
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

// TODO: at least for bool, it looks like something in cmds is catching this before us too
// ^ needs trace to find out where, probably cmds.Run or Execute; if redundant remove all these
// TODO: [general] duplicate arg type checking everywhere 😪
// figure out the best way to abstract this
// we should only have to check them in pre|post(local) + run(remote), not all 3
func unmountPreRun(request *cmds.Request, env cmds.Environment) error {
	var closeAll bool // `--all`
	if allFlag, provided := request.Options[unmountAllOptionKwd]; provided {
		if flag, isBool := allFlag.(bool); isBool {
			closeAll = flag
		} else {
			return paramError(unmountAllOptionKwd, allFlag, closeAll)
		}
	}

	if closeAll && len(request.Arguments) != 0 {
		return cmds.Errorf(cmds.ErrClient, "ambiguous request; close-all flag present alongside specific arguments: %s",
			strings.Join(request.Arguments, ", "))
	}

	return nil
}

func unmountRun(request *cmds.Request, emitter cmds.ResponseEmitter, env cmds.Environment) error {
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
				err = emitter.Emit(instance) // TODO emitter's error
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
