package mountcmds

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	cmds "github.com/ipfs/go-ipfs-cmds"
	config "github.com/ipfs/go-ipfs-config"
	"github.com/ipfs/go-ipfs/core"
	"github.com/ipfs/go-ipfs/filesystem/mount"
	"github.com/multiformats/go-multiaddr"
)

/*
We have to parse 3 different sources of strings; in priority order they are
1) command line 2) config file 3) platform recommended fallback
the command line flags, could come from 1 of 2 commands, `ipfs mount` or `ipfs daemon`
which use the same keywords except prefixed in the case of `daemon`
e.g. `ipfs mount --target="/path"` == `ipfs daemon --mount --mount-target="/path"`
*/

// TODO: the entire cmds package needs a pass, it was barely touched in the mount interface change
// this includes `cmd/ipfs/daemon.go` and `core/core.go`

const (
	cmdProviderKwd  = "provider"
	cmdNamespaceKwd = "namespace"
	cmdPathKwd      = "target"

	cmdProviderDesc  = "The underlying provider API to use for the namespace(s). Defaults to config setting or platform appropriate value."
	cmdNamespaceDesc = "A comma separated list of namespace to operate on. Defaults to config setting or platform appropriate value/"
	cmdPathDesc      = "A comma separated list of path to use. Defaults to config setting or platform appropriate value."

	cmdDaemonMountDesc  = "Mounts IPFS namespaces to the filesystem"
	cmdDaemonDescPrefix = "(if using --mount) "

	daemonCmdMountKwd     = "mount"
	daemonCmdMountPrefix  = daemonCmdMountKwd + "-"
	daemonCmdProviderKwd  = daemonCmdMountPrefix + cmdProviderKwd
	daemonCmdNamespaceKwd = daemonCmdMountPrefix + cmdNamespaceKwd
	daemonCmdTargetKwd    = daemonCmdMountPrefix + cmdPathKwd

	mountCmd  requestType = false
	daemonCmd             = true

	// TODO: templateRoot = (*nix) `/` || (NT) ${CurDrive}:\ || (any others)...
	templateHome = "IPFS_HOME"
)

var (
	cmdSharedOpts = []cmds.Option{
		cmds.StringOption(cmdProviderKwd, cmdProviderDesc),
		cmds.StringOption(cmdNamespaceKwd, cmdNamespaceDesc),
		cmds.StringOption(cmdPathKwd, cmdPathDesc),
	}

	errParamNotProvided   = errors.New("parameter was not provided")
	errConfigNotProviding = errors.New("config does not provide requested value")
)

const fuseOptSeperator = string(0x1F) // ASCII unit seperator

// keep this as is in case we want to extend this later
// if we switch to an int enum nobody has to change anything except the parseRequest logic
type requestType bool

type transformFunc func(string) string

func parseRequest(daemonRequest requestType, req *cmds.Request, nodeConf *config.Config) (mount.ProviderType, []mount.Request, error) {
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
	var provider mount.ProviderType
	if providerString, found := req.Options[transform(cmdProviderKwd)].(string); found {
		pt, err := mount.ParseProvider(providerString)
		if err != nil {
			return pt, nil, err
		}
		provider = pt
	} else {
		provider = mount.SuggestedProvider()
	}

	// --namespace=
	namespaces, err := parseNamespace(req, transform, nodeConf)
	if err != nil {
		return provider, nil, err
	}

	// --target=
	targets, err := parseTarget(req, transform, nodeConf, namespaces)
	if err != nil {
		return provider, nil, err
	}

	targetCollections, err := combine(provider, namespaces, targets)
	if err != nil {
		return provider, nil, err
	}

	return provider, targetCollections, nil
}

func MountNode(res cmds.ResponseEmitter, daemon *core.IpfsNode, provider mount.ProviderType, requests ...mount.Request) error {
	if len(requests) == 0 {
		return errors.New("no targets provided")
	}

	if !daemon.IsDaemon { // print message and block
		// FIXME: pretty print requests; lost in port
		//res.Emit(fmt.Sprintf("mounting %s in the foreground...", requests.String()))
		return daemon.Mount.Bind(provider, requests...)
	}

	// attempt mount and return
	if err := daemon.Mount.Bind(provider, requests...); err != nil {
		return err
	}

	// TODO: we should replace targets.String with an actual return from the instance
	// when using FUSE on Windows, the target `/ipfs` could be mapped to various places
	// `\\ipfs`, `\\ipfs\ipfs`, `I:\`, `I:\ipfs`, etc
	//cmds.EmitOnce(res, fmt.Sprintf("mounted: %s", requests.String()))
	//FIXME: pretty print requests; lost in port
	return nil
}

func parseNamespace(req *cmds.Request, t transformFunc, nodeConf *config.Config) ([]mount.Namespace, error) {
	// use args if provided
	namespaces, err := parseNamespaceArgs(req, t)
	if err == errParamNotProvided {
		// otherwise pull from config
		namespaces, err = parseNamespaceConfig(nodeConf)
		if err == errConfigNotProviding {
			//  otherwise fallback to suggestions
			namespaces = mount.SuggestedNamespaces()
			err = nil
		}
	}

	// expand convenience case
	if len(namespaces) == 1 && namespaces[0] == mount.NamespaceAll {
		namespaces = []mount.Namespace{mount.NamespaceIPFS, mount.NamespaceIPNS, mount.NamespaceFiles}
	}

	return namespaces, err
}

func parseNamespaceArgs(req *cmds.Request, t transformFunc) ([]mount.Namespace, error) {
	if namespaceString, found := req.Options[t(cmdNamespaceKwd)].(string); found {
		namespaceStrings, err := csv.NewReader(strings.NewReader(namespaceString)).Read()
		if err != nil {
			return nil, err
		}

		var namespaces []mount.Namespace
		for _, ns := range namespaceStrings {
			typedNs, err := mount.ParseNamespace(ns)
			if err != nil {
				return nil, err
			}
			namespaces = append(namespaces, typedNs)
		}

		return namespaces, nil
	}
	return nil, errParamNotProvided
}

// TODO: config structure around this has not be defined
func parseNamespaceConfig(nodeConf *config.Config) ([]mount.Namespace, error) {
	return nil, errConfigNotProviding
}

func parseTarget(req *cmds.Request, t transformFunc, nodeConf *config.Config, namespaces []mount.Namespace) ([]string, error) {
	// use args if provided
	targets, err := parseTargetArgs(req, t)
	if err == errParamNotProvided {
		// otherwise pull from config
		targets, err = parseTargetConfig(nodeConf, namespaces)
		if err == errConfigNotProviding {
			//  otherwise fallback to suggestions
			targets = mount.SuggestedTargets()
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

func parseTargetConfig(nodeConf *config.Config, namespaces []mount.Namespace) ([]string, error) {
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
		// TODO: separate IPFS/PinFS in this context; same for now
		case mount.NamespaceIPFS, mount.NamespacePinFS:
			if runtime.GOOS == "windows" && defaultConfig.Mounts.IPFS == nodeConf.Mounts.IPFS {
				targets = append(targets, mount.MountRoot()+"ipfs")
				continue
			}
			targets = append(targets, nodeConf.Mounts.IPFS)

		// TODO: separate IPNS/KeyFS in this context; same for now
		case mount.NamespaceIPNS, mount.NamespaceKeyFS:
			if runtime.GOOS == "windows" && defaultConfig.Mounts.IPNS == nodeConf.Mounts.IPNS {
				targets = append(targets, mount.MountRoot()+"ipns")
				continue
			}
			targets = append(targets, nodeConf.Mounts.IPNS)
		case mount.NamespaceFiles:
			// TODO: config value default + platform portability
			targets = append(targets, mount.MountRoot()+"file")
		case mount.NamespaceCombined:
			// TODO: config value default + platform portability
			targets = append(targets, mount.SuggestedCombinedPath())
		default:
			return nil, fmt.Errorf("unexpected namespace: %s", ns.String())
		}
	}
	return targets, nil
}

func combine(provider mount.ProviderType, namespaces []mount.Namespace, targets []string) ([]mount.Request, error) {
	if tLen, nLen := len(targets), len(namespaces); tLen != nLen {
		return nil, fmt.Errorf("namespace and target count do not match([%d]namespaces:%v|[%d]targets:%v)", nLen, namespaces, tLen, targets)
	}

	var requests []mount.Request
	for i, t := range targets {
		var providerParam string
		switch provider {
		case mount.ProviderPlan9Protocol:
			var err error
			if providerParam, err = nineArgs(&t, namespaces[i]); err != nil {
				return nil, err
			}
		case mount.ProviderFuse:
			providerParam = fuseArgs(&t, namespaces[i])
		}

		requests = append(requests,
			mount.Request{Namespace: namespaces[i], Target: t, Parameter: providerParam},
		)
	}
	return requests, nil
}

// modifies the request target (if necessary for the platform/request)
// and may return a 9P specific parameter
// if the target is a file system path, to be mounted by us (as a client)
// the parameter string will specify a maddr for the provider to use (as the server)
// if the target is itself a listener address, it will be moved to the parameter string, and the target cleared
// (despite its name, the arity of `nineArgs` is 2, not 9)
func nineArgs(target *string, namespace mount.Namespace) (string, error) {
	// we allow templating unix domain socket maddrs, so check for those and expand them here
	if strings.HasPrefix(*target, "/unix") {
		listenerString, err := stabilizeUnixPath(*target)
		if err != nil {
			return "", err
		}
		*target = listenerString
	}

	// if the target is a maddr, move it to the parameter string
	// to signify to the provider there is no mount target, and we just want the listener
	if _, err := multiaddr.NewMultiaddr(*target); err == nil {
		listenerString := *target
		*target = ""
		return listenerString, nil
	}
	// otherwise, provide a listening address for targets that are themselves, not-listeners
	// leaving the target string unmodified
	return stabilizeUnixPath(fmt.Sprintf("/unix/$IPFS_HOME/9p.%s.sock", namespace.String()))
}

// modifies the request target (if necessary for the platform/request)
// and may return a platform local, FUSE library specific, parameter array
// the array is joined and delimited by the ASCII unit seperator before being returned
// and should be expanded back into a string array for use with `fuselib.Mount`'s `opts` parameter
func fuseArgs(target *string, namespace mount.Namespace) string {
	var (
		opts string
		args []string
	)

	switch runtime.GOOS {
	default:
		// NOOP

	case "windows": // expected target is WinFSP; use its options
		// cgofuse expects an argument format comprised of components
		// e.g. `mount.exe -o "uid=-1,volname=a valid name,gid=-1" --VolumePrefix=\localhost\UNC`
		// is equivalent to this in Go:
		//`[]string{"-o", "uid=-1,volname=a valid name,gid=-1", "--VolumePrefix=\\localhost\\UNC"}`
		// refer to the WinFSP documentation for expected parameters and their literal format

		// basic info
		if namespace == mount.NamespaceCombined {
			opts = "FileSystemName=IPFS,volname=IPFS"
		} else {
			opts = fmt.Sprintf("FileSystemName=%s,volname=%s", namespace.String(), namespace.String())
		}
		// set the owner to be the same as the process (`daemon`'s or `mount`'s depending on background/foreground)
		opts += ",uid=-1,gid=-1"
		args = append(args, "-o", opts)

		// convert UNC targets to WinFSP format
		if len(*target) > 2 && (*target)[:2] == `\\` {
			// NOTE: cgo-fuse/WinFSP UNC parameter uses single slash prefix, so we chop one off
			args = append(args, fmt.Sprintf(`--VolumePrefix=%s`, (*target)[1:]))
			*target = "" // unset target value; UNC is handled by `VolumePrefix`
		}
		// otherwise target is another reference; leave it alone

	case "freebsd":
		if namespace == mount.NamespaceCombined {
			opts = "fsname=IPFS,subtype=IPFS"
		} else {
			opts = fmt.Sprintf("fsname=%s,subtype=%s", namespace.String(), namespace.String())
		}

		// TODO: [general] we should allow the user to pass in raw options
		// that we will then relay to the underlying fuse implementation, unaltered
		// options like `allow_other` depend on opinions of the sysop, not us
		// so we shouldn't just assume this is what they want
		if os.Geteuid() == 0 { // if root, allow other users to access the mount
			opts += ",allow_other" // allow users besides root to see and access the mount

			//opts += ",default_permissions"
			// TODO: [cli, constructors]
			// for now, `default_permissions` won't prevent anything
			// since we tell whoever is calling that they own the file, regardless of who it is
			// we need a way for the user to set `uid` and `gid` values
			// both for our internal context (getattr)
			// as well as allowing them to pass the uid= and gid= FUSE options (not specifically, pass anything)
			// (^system ignores our values and substitutes its own)
		}
		args = append(args, "-o", opts)

	case "openbsd":
		args = append(args, "-o", "allow_other")

	case "darwin":
		if namespace == mount.NamespaceCombined {
			// TODO: see if we can provide `volicon` via an IPFS path; or make the overlay provide one via `/.VolumeIcon.icns` on darwin
			opts = "fsname=IPFS,volname=IPFS"
		} else {
			opts = fmt.Sprintf("fsname=%s,volname=%s", namespace.String(), namespace.String())
		}

		args = append(args, "-o", opts)

		// TODO reconsider if we should leave this hack in
		// macfuse takes this literally and will make a mountpoint as `./~/target` not `/home/user/target`
		if strings.HasPrefix(*target, "~") {
			usr, err := user.Current()
			if err != nil {
				panic(err)
			}
			*target = usr.HomeDir + (*target)[1:]
		}

	case "linux":
		// [2020.04.18] cgofuse currently backed by hanwen/go-fuse on linux
		// their option set doesn't support our desired options
		// libfuse: opts = fmt.Sprintf(`-o fsname=ipfs,subtype=fuse.%s`, namespace.String())
	}

	return strings.Join(args, fuseOptSeperator)
}

func stabilizeUnixPath(maString string) (string, error) {
	templateValueRepoPath, err := config.PathRoot()
	if err != nil {
		return "", err
	}

	if !filepath.IsAbs(templateValueRepoPath) { // stabilize root path
		absRepo, err := filepath.Abs(templateValueRepoPath)
		if err != nil {
			return "", err
		}
		templateValueRepoPath = absRepo
	}

	// expand templates

	// NOTE: the literal parsing and use of `/` is interned
	// we don't want to treat this like a file system path, it is specifically a multiaddr string
	// this prevents the template from expanding to double slashed paths like `/unix//home/...` on *nix systems
	// but allow it to expand to `/unix/C:\Users\...` on NT, which is the valid form for the maddr target value
	templateValueRepoPath = strings.TrimPrefix(templateValueRepoPath, "/")

	// only expand documented template keys, not everything
	return os.Expand(maString, func(key string) string {
		return (map[string]string{
			templateHome: templateValueRepoPath,
		})[key]
	}), nil
}

/* TODO: [lint] we likely won't need this; maybe the warning for length would be good to have
const sun_path_len = 108
// TODO: multiaddr encapsulation concerns; this is just going to destroy every socket, not just ours
// it should probably just operate on the final component
func removeUnixSockets(maddr multiaddr.Multiaddr) error {
	var retErr error

	multiaddr.ForEach(maddr, func(comp multiaddr.Component) bool {
		if comp.Protocol().Code == multiaddr.P_UNIX {
			target := comp.Value()
			if runtime.GOOS == "windows" {
				target = strings.TrimLeft(target, "/")
			}
			if len(target) >= sun_path_len {
				// TODO [anyone] this type of check is platform dependant and checks+errors around it should exist in `mulitaddr` when forming the actual structure
				// e.g. on Windows 1909 and lower, this will always fail when binding
				// on Linux this can cause problems if applications are not aware of the true addr length and assume `sizeof addr <= 108`

				// FIXME: we lost our logger in the port from plugin; this shouldn't use fmt
				// logger.Warning("Unix domain socket path is at or exceeds standard length `sun_path[108]` this is likely to cause problems")
				fmt.Printf("[WARNING] Unix domain socket path %q is at or exceeds standard length `sun_path[108]` this is likely to cause problems\n", target)
			}

			// discard notexist errors
			if callErr := os.Remove(target); callErr != nil && !os.IsNotExist(callErr) {
				retErr = callErr
				return false // break out of ForEach
			}
		}
		return true // continue
	})

	return retErr
}
*/
