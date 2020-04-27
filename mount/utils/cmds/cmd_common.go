package mountcmds

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io/ioutil"
	"runtime"
	"strings"

	cmds "github.com/ipfs/go-ipfs-cmds"
	config "github.com/ipfs/go-ipfs-config"
	"github.com/ipfs/go-ipfs/core"
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
	daemonCmdMountPrefix  = daemonCmdMountKwd + "-"
	daemonCmdProviderKwd  = daemonCmdMountPrefix + cmdProviderKwd
	daemonCmdNamespaceKwd = daemonCmdMountPrefix + cmdNamespaceKwd
	daemonCmdTargetKwd    = daemonCmdMountPrefix + cmdPathKwd
)

var cmdSharedOpts = []cmds.Option{
	cmds.StringOption(cmdProviderKwd, cmdProviderDesc),
	cmds.StringOption(cmdNamespaceKwd, cmdNamespaceDesc),
	cmds.StringOption(cmdPathKwd, cmdPathDesc),
}

var (
	errParamNotProvided   = errors.New("parameter was not provided")
	errConfigNotProviding = errors.New("config does not provide requested value")
)

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
	// priority: args > conf > platform specific suggestions

	// TODO: define our new values in the config structure + parser + init
	// (Mounts.Provider; Mounts.Namespace; Mounts.Files; Mounts.Target?)
	// right now we can't pull from undefined values obviously

	// daemon requests are the same as mount requests, except prefixed
	// we'll translate the parameters to match when parsing
	var transform transformFunc
	if daemonRequest {
		transform = func(param string) string { return daemonCmdMountPrefix + param }
	} else {
		transform = func(param string) string { return param }
	}

	// --provider=
	var provider mountinter.ProviderType
	if providerString, found := req.Options[transform(cmdProviderKwd)].(string); found {
		provider = mountinter.ParseProvider(providerString)
	} else {
		provider = mountinter.SuggestedProvider()
	}

	// --namespace=
	namespaces, err := parseNamespace(req, transform, nodeConf)
	if err != nil {
		return mountinter.ProviderNone, nil, err
	}

	// --target=
	targets, err := parseTarget(req, transform, nodeConf, namespaces)
	if err != nil {
		return mountinter.ProviderNone, nil, err
	}

	targetCollections, err := combine(provider, namespaces, targets)
	if err != nil {
		return mountinter.ProviderNone, nil, err

	}

	return provider, targetCollections, nil
}

func MountNode(res cmds.ResponseEmitter, daemon *core.IpfsNode, provider mountinter.ProviderType, targets mountinter.TargetCollections) error {
	if len(targets) == 0 {
		return errors.New("no targets provided")
	}

	if !daemon.IsDaemon { // print message and block
		res.Emit(fmt.Sprintf("mounting %s in the foreground...", targets.String()))
		return daemon.Mount.Graft(provider, targets)
	}

	// attempt mount and return
	if err := daemon.Mount.Graft(provider, targets); err != nil {
		return err
	}

	// TODO: we should replace targets.String with an actual return from the instance
	// when using FUSE on Windows, the target `/ipfs` could be mapped to various places
	// `\\ipfs`, `\\ipfs\ipfs`, `I:\`, `I:\ipfs`, etc
	cmds.EmitOnce(res, fmt.Sprintf("mounted: %s", targets.String()))
	return nil
}

func parseNamespace(req *cmds.Request, t transformFunc, nodeConf *config.Config) ([]mountinter.Namespace, error) {
	// use args if provided
	namespaces, err := parseNamespaceArgs(req, t)
	if err == errParamNotProvided {
		// otherwise pull from config
		namespaces, err = parseNamespaceConfig(nodeConf)
		if err == errConfigNotProviding {
			//  otherwise fallback to suggestions
			namespaces = mountinter.SuggestedNamespaces()
			err = nil
		}
	}

	// expand convenience case
	if len(namespaces) == 1 && namespaces[0] == mountinter.NamespaceAll {
		namespaces = []mountinter.Namespace{mountinter.NamespaceIPFS, mountinter.NamespaceIPNS, mountinter.NamespaceFiles}
	}

	return namespaces, err
}

func parseNamespaceArgs(req *cmds.Request, t transformFunc) ([]mountinter.Namespace, error) {
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
	return nil, errParamNotProvided
}

// TODO: config structure around this has not be defined
func parseNamespaceConfig(nodeConf *config.Config) ([]mountinter.Namespace, error) {
	return nil, errConfigNotProviding
}

func parseTarget(req *cmds.Request, t transformFunc, nodeConf *config.Config, namespaces []mountinter.Namespace) ([]string, error) {
	// use args if provided
	targets, err := parseTargetArgs(req, t)
	if err == errParamNotProvided {
		// otherwise pull from config
		targets, err = parseTargetConfig(nodeConf, namespaces)
		if err == errConfigNotProviding {
			//  otherwise fallback to suggestions
			targets = mountinter.SuggestedTargets()
			if len(targets) != len(namespaces) {
				return targets, errors.New("platform target defaults clash with provided namespace, please specify both namespace and target parameters")
			}
			err = nil
		}
	}

	return targets, err
}

func parseTargetArgs(req *cmds.Request, t transformFunc) ([]string, error) {
	if targetString, found := req.Options[t(cmdPathKwd)].(string); found {
		targets, err := csv.NewReader(strings.NewReader(targetString)).Read()
		if err != nil {
			return nil, err
		}
		return targets, nil
	}
	return nil, errParamNotProvided
}

func parseTargetConfig(nodeConf *config.Config, namespaces []mountinter.Namespace) ([]string, error) {
	var targets []string

	// TODO: config defaults have to change to some kind of portable format
	// ideally a templated value like `${mountroot}ipfs`, `${mountroot}ipns`, etc.
	// until then we have this nasty hack
	defaultConfig, err := config.Init(ioutil.Discard, 2048)
	if runtime.GOOS == "windows" {
		if err != nil {
			return nil, err
		}
	}
	for _, ns := range namespaces {
		switch ns {
		case mountinter.NamespaceIPFS:
			if runtime.GOOS == "windows" && defaultConfig.Mounts.IPFS == nodeConf.Mounts.IPFS {
				targets = append(targets, mountinter.MountRoot()+"ipfs")
				continue
			}
			targets = append(targets, nodeConf.Mounts.IPFS)
		case mountinter.NamespaceIPNS:
			if runtime.GOOS == "windows" && defaultConfig.Mounts.IPNS == nodeConf.Mounts.IPNS {
				targets = append(targets, mountinter.MountRoot()+"ipns")
				continue
			}
			targets = append(targets, nodeConf.Mounts.IPNS)
		case mountinter.NamespaceFiles:
			// TODO: config value default + platform portability
			targets = append(targets, mountinter.MountRoot()+"file")
		case mountinter.NamespaceAllInOne:
			// TODO: config value default + platform portability
			targets = append(targets, mountinter.SuggestedAllInOnePath())
		default:
			return nil, fmt.Errorf("unexpected namespace: %s", ns.String())
		}
	}
	return targets, nil
}

func combine(provider mountinter.ProviderType, namespaces []mountinter.Namespace, targets []string) ([]mountinter.TargetCollection, error) {
	if tLen, nLen := len(targets), len(namespaces); tLen != nLen {
		return nil, fmt.Errorf("namespace and target count do not match(%d|%d)", tLen, nLen)
	}

	var collections mountinter.TargetCollections
	for i, t := range targets {
		var providerParam string
		switch provider {
		case mountinter.ProviderPlan9Protocol:
			providerParam = fmt.Sprintf("/unix/$IPFS_HOME/9p.%s.sock", namespaces[i].String())
		}

		collections = append(collections,
			mountinter.TargetCollection{Namespace: namespaces[i], Target: t, Parameter: providerParam},
		)
	}
	return collections, nil
}
