package main

import (
	"context"
	"errors"
	"log"
	"os"

	"github.com/ipfs/go-ipfs-cmds/cli"
	fscmds "github.com/ipfs/go-ipfs/core/commands/filesystem"
)

func main() {
	log.SetFlags(log.Lshortfile | log.Ltime)
	//log.SetOutput(io.Discard)
	//log.SetOutput(os.Stderr)
	//log.Println(os.Args)

	var (
		ctx = context.Background()
		err = cli.Run(ctx, fscmds.FullRoot, os.Args, // pass in command and args to parse
			os.Stdin, os.Stdout, os.Stderr, // along with output writers
			fscmds.MakeFileSystemEnvironment, fscmds.MakeFileSystemExecutor) // and our constructor+receiver pair
	)

	cliError := new(cli.ExitError)
	if errors.As(err, cliError) {
		os.Exit(int((*cliError)))
	}
}
