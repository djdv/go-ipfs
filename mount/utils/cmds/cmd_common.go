package mountcmds

import (
	"encoding/csv"
	"errors"
	"fmt"
	"strings"

	cmds "github.com/ipfs/go-ipfs-cmds"
	config "github.com/ipfs/go-ipfs-config"
	"github.com/ipfs/go-ipfs/core"
	con "github.com/ipfs/go-ipfs/mount/conductors"
	mountinter "github.com/ipfs/go-ipfs/mount/interface"
)

/* XXX: this entire file is gross; try not to look at it
we have to parse 3 different sources of strings; in priority order they are
1) command line 2) config file 3) platform reccomended fallback
the command line flags, could be 1 of 2 sets, from the command `ipfs mount` or `ipfs daemon`
which use different parameter keywords depending on the command invoked
e.g. `ipfs mount --ipfs-path="/path"` == `ipfs daemon --mount --mount-ipfs="/path"`
we then transmogrify all that into a psuedo associative array so we can parse it into static typed values
*/

const (
	/* TODO: [discuss] ask if we can break compat so we can employ a consistent prefix pattern
	daemon commands should be EXACTLY the same as the mount/unmount commands
	just prefixed with `--mount`
	i.e. `daemon --mount --mount-ipfs-path`, not `daemon --mount --mount-ipfs`
	to match `ipfs mount --ipfs-path=`

	this would make parsing easier (a lot less redundancy) and be friendlier to people writing scripts
	(so they don't have to remember 2 parameters instead of 1 + a subcommand prefix)
	*/

	cmdProviderKwd  = "provider"
	cmdNamespaceKwd = "namespace"
	cmdPathKwd      = "target"

	cmdProviderDesc  = "The underlying provider API to use for the namespace(s). Defaults to config setting or platform appropriate value."
	cmdNamespaceDesc = "A comma seperated list of namespace to operate on. Defaults to config setting or platform appropriate value/"
	cmdPathDesc      = "A comma seperated list of path to use. Defaults to config setting or platform appropriate value."

	cmdDaemonMountDesc  = "Mounts IPFS namespaces to the filesystem"
	cmdDaemonDescPrefix = "(if using --mount) "

	daemonCmdMountKwd     = "mount"
	daemonCmdProviderKwd  = "mount-" + cmdProviderKwd
	daemonCmdNamespaceKwd = "mount-" + cmdNamespaceKwd
	daemonCmdTargetKwd    = "mount-" + cmdPathKwd
)

var cmdSharedOpts = []cmds.Option{
	cmds.StringOption(cmdProviderKwd, cmdProviderDesc),
	cmds.StringOption(cmdNamespaceKwd, cmdNamespaceDesc),
	cmds.StringOption(cmdPathKwd, cmdPathDesc),
}

// keep this as is in case we want to extend this later
// if we switch to an int enum nobody has to change anything except the parseRequest logic
type requestType bool

const (
	mountCmd  requestType = false
	daemonCmd             = true
)

type transformFunc func(string) string

func parseRequest(daemonRequest requestType, req *cmds.Request, nodeConf *config.Config) (mountinter.ProviderType, mountinter.TargetCollections, error) {
	// parse flags if provided, otherwise fallback to config values

	// TODO: define our new values in the config structure + parser + init
	// (Mounts.Provider; Mounts.Namespace; Mounts.Files; Mounts.Target?)
	// right now we can't pull from undefined values obviously

	// priority: args > conf > suggestion

	var transform transformFunc
	if daemonRequest {
		transform = func(param string) string { return "mount-" + param }
	} else {
		transform = func(param string) string { return param }
	}

	var provider mountinter.ProviderType
	if providerString, found := req.Options[transform(cmdProviderKwd)].(string); found {
		provider = mountinter.ParseProvider(providerString)
	} else {
		provider = mountinter.SuggestedProvider()
	}

	namespaces, err := parseNamespace(req, transform)
	if err != nil {
		return mountinter.ProviderNone, nil, err
	}

	if len(namespaces) == 1 {
		if namespaces[0] == mountinter.NamespaceAll {
			// expand special case
			namespaces = []mountinter.Namespace{mountinter.NamespaceIPFS, mountinter.NamespaceIPNS, mountinter.NamespaceFiles}
		}
	}

	targetCollections, err := parseTarget(req, transform, nodeConf, namespaces)
	if err != nil {
		return mountinter.ProviderNone, nil, err
	}

	appendProviderParameters(provider, targetCollections)

	return provider, targetCollections, nil
}

func appendProviderParameters(provider mountinter.ProviderType, targets mountinter.TargetCollections) {
	if provider == mountinter.ProviderPlan9Protocol {
		for i, target := range targets {
			//TODO: [consider] we could template namespace and let the provider handle it ${API_NAMESPACE}
			targets[i].Parameter = fmt.Sprintf("/unix/$IPFS_HOME/9p.%s.sock", target.Namespace.String())
		}
	}
}

func MountNode(res cmds.ResponseEmitter, daemon *core.IpfsNode, provider mountinter.ProviderType, targets mountinter.TargetCollections) error {
	if len(targets) == 0 {
		return errors.New("no targets provided")
	}

	conOps := []con.Option{con.MountFilesRoot(daemon.FilesRoot)}

	if !daemon.IsDaemon { // print message and block
		return errors.New("mounting in foreground not supported yet")

		// WIP:
		conOps = append(conOps, con.MountForeground(true))
		res.Emit(fmt.Sprintf("mounting %s in the foreground...", targets.String()))
		return daemon.Mount.Graft(provider, targets, conOps...)
	}

	// attempt mount and return
	if err := daemon.Mount.Graft(provider, targets, conOps...); err != nil {
		return err
	}

	// TODO: we should replace targets.String with an actual return from the instance
	// when using FUSE on Windows, the target `/ipfs` could be mapped to various places
	// `\\ipfs`, `\\ipfs\ipfs`, `I:\`, `I:\ipfs`, etc
	cmds.EmitOnce(res, fmt.Sprintf("mounted: %s", targets.String()))
	return nil
}

func parseNamespace(req *cmds.Request, t transformFunc) ([]mountinter.Namespace, error) {
	// use args if provided
	if namespaceString, found := req.Options[t(cmdNamespaceKwd)].(string); found {
		namespaceStrings, err := csv.NewReader(strings.NewReader(namespaceString)).Read()
		if err != nil {
			return nil, err
		}

		var namespaces []mountinter.Namespace
		for _, ns := range namespaceStrings {
			namespaces = append(namespaces, mountinter.ParseNamespace(ns))
		}
		return namespaces, nil

	}

	// TODO: pull from config values here

	// fallback to suggestions
	return []mountinter.Namespace{mountinter.SuggestedNamespace()}, nil
}

func parseTarget(req *cmds.Request, t transformFunc, nodeconf *config.Config, namespaces []mountinter.Namespace) (mountinter.TargetCollections, error) {
	// use args if provided
	if targetString, found := req.Options[t(cmdPathKwd)].(string); found {
		targets, err := csv.NewReader(strings.NewReader(targetString)).Read()
		if err != nil {
			return nil, err
		}

		if tLen, nLen := len(targets), len(namespaces); tLen != nLen {
			return nil, fmt.Errorf("namespace and target count to not match(%d|%d)", tLen, nLen)
		}

		var collections mountinter.TargetCollections
		for i, t := range targets {
			collections = append(collections, mountinter.TargetCollection{Namespace: namespaces[i], Target: t})
		}

		return collections, nil
	}

	// pull targets from config
	var collections mountinter.TargetCollections
	for _, ns := range namespaces {
		var targ string

		// TODO: we need to modify the config to append files and maybe AIO
		// in any case we need to inspect the values to make sure they're both 1) not empty 2) platform independent
		// something like templates might help e.g. ${ROOT}mnt rather than /mnt; on Windows this would resolve to `$current-vol\mnt`
		// as is, the defaults are not great on Windows
		switch ns {
		case mountinter.NamespaceIPFS:
			targ = nodeconf.Mounts.IPFS
		case mountinter.NamespaceIPNS:
			targ = nodeconf.Mounts.IPNS
		case mountinter.NamespaceFiles:
			targ = "/file"
		case mountinter.NamespaceAllInOne:
			targ = "/mnt"
		default:
			return nil, fmt.Errorf("unexpected namespace: %v", ns)
		}

		collections = append(collections, mountinter.TargetCollection{Namespace: ns, Target: targ})
	}
	return collections, nil
}
