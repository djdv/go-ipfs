package fscmds

import (
	"os"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs-cmds/cli"
)

const daemonPostRunType cmds.PostRunType = "daemon"

// wraps regular cli emitter but with a distinct PostRunType
// which formats the responses for the daemon's standard outputs
type daemonEmitter struct{ cmds.ResponseEmitter }

func (daemonEmitter) Type() cmds.PostRunType { return daemonPostRunType }

func DaemonEmitter(req *cmds.Request) (em cmds.ResponseEmitter, err error) {
	em, err = cli.NewResponseEmitter(os.Stdout, os.Stderr, req)
	if err != nil {
		return
	}
	em = daemonEmitter{em}
	return
}
