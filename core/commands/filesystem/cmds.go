package fscmds

import (
	"strings"

	cmds "github.com/ipfs/go-ipfs-cmds"
	config "github.com/ipfs/go-ipfs-config"
)

const (
	mountPathIPFSKwd      = "ipfs-path"
	mountPathIPFSKwdShort = "f"
	mountPathIPFSDesc     = "The path where IPFS should be mounted."
	mountPathIPNSKwd      = "ipns-path"
	mountPathIPNSKwdShort = "n"
	mountPathIPNSDesc     = "The path where IPNS should be mounted."

	// Commands that embed our Command, should use our parameters (prefixed)
	// So as to maintain parameter name parity
	// `ipfs mount --ipfs-path=/x`
	// `ipfs daemon --mount --mount-ipfs-path=/x`
	mountPrefix = "mount"
)

var baseOpts = []cmds.Option{
	cmds.StringOption(mountPathIPFSKwd, mountPathIPFSKwdShort, mountPathIPFSDesc),
	cmds.StringOption(mountPathIPNSKwd, mountPathIPNSKwdShort, mountPathIPNSDesc),
}

func parseRequest(req *cmds.Request, defaults config.Mounts) (string, string, error) {
	const paramErrStr = "failed to get %s parameter arguments from request: %w"

	// if present, translate `ipfs daemon` request parameters to `ipfs mount` parameters
	if prefixFlag, _ := req.Options[mountPrefix].(bool); prefixFlag {
		req.Options = clipPrefix(mountPrefix, req.Options)
	}

	// use args if provided, config otherwise
	ipfsTarget, found := req.Options[mountPathIPFSKwd].(string)
	if !found {
		ipfsTarget = defaults.IPFS
	}
	ipnsTarget, found := req.Options[mountPathIPNSKwd].(string)
	if !found {
		ipnsTarget = defaults.IPNS
	}

	return ipfsTarget, ipnsTarget, nil
}

// clipPrefix returns an optmap, with parameter names that match the inputs
// without their prefix.
// e.g.
// `program cmd --prefix --prefix-parameter=value` is remapped as if it were
// `program cmd --parameter=value`.
func clipPrefix(prefix string, nodeOpts cmds.OptMap) cmds.OptMap {
	trimmedOpts := make(cmds.OptMap, len(nodeOpts)) // NOTE: we don't want to modify the source map
	for param, arg := range nodeOpts {
		switch {
		default:
			// NOOP; don't copy options that don't apply to us
			// (except the encoding option if present)
		case param == cmds.EncLong || param == cmds.EncShort:
			trimmedOpts[param] = arg

		case param == prefix:
			// don't copy the prefix itself

		case strings.HasPrefix(param, prefix): // copy prefixed parameters; sans-prefix
			trimmedOpts[strings.TrimPrefix(param, prefix+"-")] = arg
			// ^ e.g. `superCmd --prefix-ABC=123` => `cmd -ABC=123`
		}
	}

	return trimmedOpts
}
