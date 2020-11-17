package fscmds

import cmds "github.com/ipfs/go-ipfs-cmds"

var DaemonOpts = prependPrefix(mountPrefix, mountTagline, baseOpts)

// prependPrefix returns a copy of the input options,
// with a "prefix flag" and prefixed parameter names.
// For example: `program cmd` may have parameters added to it
// `program cmd --prefix --prefix-parameter=value` inherited from
// `program cmd2 -parameter=value`
func prependPrefix(prefix, description string, opts []cmds.Option) []cmds.Option {
	prefixedOpts := make([]cmds.Option, 0, len(opts)+1)

	// add the prefix as a flag itself: `--prefix`
	prefixedOpts = append(prefixedOpts,
		cmds.BoolOption(prefix, description),
	)

	// delimiter: `--prefix` -> `--prefix-`
	paramPrefix := prefix + "-"

	for _, opt := range opts { // generate an option instance from the string definition
		prefixedOpts = append(prefixedOpts,
			cmds.NewOption(opt.Type(),
				paramPrefix+opt.Name(),                        // prefix it's `Name`; `--prefix-$Name`
				"(if using --"+prefix+") "+opt.Description()), // prefix the helptext with its message as well
		)
	}

	return prefixedOpts
}
