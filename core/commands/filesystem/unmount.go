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
)

var Unmount = &cmds.Command{
	//TODO Options: append(sharedOpts,
	//	cmds.BoolOption(unmountAllKwd, "a", unmountAllDesc)),
	Arguments: []cmds.Argument{
		cmds.StringArg("targets", true, true, MountArgumentDescription),
	},
	//PreRun: unmountPreRun,
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

func unmountRun(req *cmds.Request, re cmds.ResponseEmitter, env cmds.Environment) error {
	node, err := cmdenv.GetNode(env)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	for instance := range node.FileSystem.List(req.Context) {
		wg.Add(1)
		go func(instance manager.Response) {
			defer wg.Done()
			for _, instanceTarget := range req.Arguments {
				if instance.String() == instanceTarget {
					instance.Error = instance.Close()
					re.Emit(instance) // TODO err
				}
			}
		}(instance)
	}
	wg.Wait()
	return nil
}

func unmountPostRunCLI(response cmds.Response, emitter cmds.ResponseEmitter) (err error) {
	encType := cmds.GetEncoding(response.Request(), "")
	consoleOut, emit := printXorRelay(encType, emitter)
	decoWrite := func(s string) (err error) { _, err = consoleOut.Write([]byte(s)); return }

	decoWrite(fmt.Sprintf("Closing: %s\n",
		strings.Join(response.Request().Arguments, ", ")))

	ctx := response.Request().Context
	responses := cmdsResponseToManagerResponses(ctx, response)
	for fsResponse := range drawResponses(ctx, consoleOut, responses) {
		emit(fsResponse) // TODO: emitter errors
	}

	return
}
