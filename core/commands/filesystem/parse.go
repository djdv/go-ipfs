package fscmds

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

	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"

	"github.com/ipfs/go-ipfs/filesystem"

	cmds "github.com/ipfs/go-ipfs-cmds"
	config "github.com/ipfs/go-ipfs-config"
	"github.com/ipfs/go-ipfs/core/commands/cmdenv"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager"
	"github.com/multiformats/go-multiaddr"
)

var (
	/*
		cmdSharedOpts = []cmds.Option{
			cmds.StringOption(NamespaceKwd, cmdNamespaceDesc),
			cmds.StringOption(PathKwd, cmdPathDesc),
		}
	*/

	errParamNotProvided   = errors.New("parameter was not provided")
	errConfigNotProviding = errors.New("config does not provide requested value")
)

/*
We parse 3 different sources of strings (in priority order:)
	-) command line
	-) config file
	-) platform recommended fallback
The command line flags could come from either:
	-) `ipfs mount`
	-) `ipfs daemon`

TODO: finish text^

Flag key names are node option names (like `--target`)
Other `cmds.Command`s using this request parser,
should use a prefixed version of the `Command`
e.g. command flags should look like this:
	`ipfs filesystem --mount --target="/path"`
	(cmds alias) `ipfs mount --filesystem-target="/path"`
	    `ipfs daemon --mount --filesystem-target="/path"`
*/

// returns true if the input request was translated into a bind request
func TranslateToMountRequest(prefix string, sourceReq *cmds.Request) (*cmds.Request, bool) {
	if prefixFlag, _ := sourceReq.Options[prefix].(bool); !prefixFlag {
		return nil, false
	}

	mountReq := *sourceReq
	mountReq.Command = Mount

	delete(mountReq.Options, prefix) // delete the prefix option itself first

	for param, arg := range mountReq.Options {
		delete(mountReq.Options, param)       // delete the rest of the options
		if strings.HasPrefix(param, prefix) { // keeping (unencapsulated) copies of prefixed parameters
			// e.g. `superCmd --prefix-ABC=123` => `cmd -ABC=123`
			mountReq.Options[strings.TrimPrefix(param, prefix+"-")] = arg
		}
	}

	return &mountReq, true
}

func parseRequest(req *cmds.Request, env cmds.Environment) ([]manager.Request, error) {
	const paramErrStr = "failed to get file system parameter from request: %w"

	// --subsystem=
	subsystems, err := parseSubsystem(req, env)
	if err != nil {
		return nil, fmt.Errorf(paramErrStr, err)
	}

	// --target=
	targets, err := parseTarget(req, env, subsystems)
	if err != nil {
		return nil, fmt.Errorf(paramErrStr, err)
	}

	// --api=
	api, err := parseAPI(req)
	if err != nil {
		return nil, fmt.Errorf(paramErrStr, err)
	}

	requests, err := combine(api, subsystems, targets)
	if err != nil {
		return nil, fmt.Errorf(paramErrStr, err)
	}
	return requests, nil
}

func hasParam(req *cmds.Request, parameterName string) (string, bool) {
	subsystemString, found := req.Options[parameterName].(string)
	return subsystemString, found
}

func parseSubsystem(req *cmds.Request, env cmds.Environment) ([]filesystem.ID, error) {
	// use args if provided
	if subsystemString, paramProvided := hasParam(req, subsystemKwd); paramProvided {
		return subsystemArg(subsystemString)
	}

	// otherwise pull from config
	nodeConf, err := cmdenv.GetConfig(env)
	if err != nil {
		return nil, fmt.Errorf("failed to get file system's subsystem config from node: %w", err)
	}

	subsystems, err := subsystemConfig(nodeConf)
	if err == errConfigNotProviding {
		//  otherwise fallback to suggestions
		return subsystemArg(defaultSystemsOption)
	}

	return subsystems, err
}

func subsystemArg(arg string) ([]filesystem.ID, error) {
	idStrings, err := csv.NewReader(strings.NewReader(arg)).Read()
	if err != nil {
		return nil, err
	}

	var typedIds []filesystem.ID
	for _, idString := range idStrings {
		typedSys, err := typeCastSystemArg(idString)
		if err != nil {
			return nil, err
		}
		typedIds = append(typedIds, typedSys)
	}

	return typedIds, nil
}

// TODO: config structure around this has not be defined
func subsystemConfig(nodeConf *config.Config) ([]filesystem.ID, error) {
	return nil, errConfigNotProviding
}

func parseAPI(req *cmds.Request) ([]manager.API, error) {
	// use args if provided
	if apiString, paramProvided := hasParam(req, aPIKwd); paramProvided {
		return parseAPIArg(apiString)
	}
	return parseAPIArg(defaultAPIOption) //  otherwise fallback to suggestions
}

func parseAPIArg(arg string) ([]manager.API, error) {
	apiStrings, err := csv.NewReader(strings.NewReader(arg)).Read()
	if err != nil {
		return nil, err
	}

	var apis []manager.API
	for _, apiString := range apiStrings {
		typedAPI, err := typeCastAPIArg(apiString)
		if err != nil {
			return nil, err
		}
		apis = append(apis, typedAPI)
	}

	return apis, nil
}

func parseTarget(req *cmds.Request, env cmds.Environment, systems []filesystem.ID) ([]string, error) {
	// use args if provided
	if targetString, paramProvided := hasParam(req, TargetKwd); paramProvided {
		return parseTargetArg(targetString)
	}

	// otherwise pull from config
	nodeConf, err := cmdenv.GetConfig(env)
	if err != nil {
		return nil, fmt.Errorf("failed to get file system's target config from node: %w", err)
	}

	targets, err := parseTargetConfig(nodeConf, systems)
	switch err {
	case nil:
	case errConfigNotProviding:
		//  fallback to suggestions
		targets = defaultTargets
	default:
		return nil, err
	}

	if len(targets) != len(systems) {
		return targets, errors.New("platform target defaults clash with provided namespace, please specify both namespace and target parameters")
	}

	return targets, nil
}

func parseTargetArg(arg string) ([]string, error) {
	targets, err := csv.NewReader(strings.NewReader(arg)).Read()
	if err != nil {
		return nil, err
	}
	return targets, nil
}

func parseTargetConfig(nodeConf *config.Config, systems []filesystem.ID) ([]string, error) {
	var targets []string

	// TODO: config defaults have to change to some kind of portable format
	// ideally a templated value like `${mountroot}ipfs`, `${mountroot}ipns`, etc.
	// until then we have this nasty hack:
	// if the default value exists, we substitute our own value that is more appropriate to the platform
	defaultConfig, err := config.Init(ioutil.Discard, 2048)
	if runtime.GOOS == "windows" {
		if err != nil {
			return nil, err
		}
	}

	for _, system := range systems {
		switch system {
		// TODO: separate IPFS/PinFS in this context; same for now
		case filesystem.IPFS, filesystem.PinFS:
			if runtime.GOOS == "windows" && defaultConfig.Mounts.IPFS == nodeConf.Mounts.IPFS {
				targets = append(targets, platformMountRoot+"ipfs")
				continue
			}
			targets = append(targets, nodeConf.Mounts.IPFS)

		// TODO: separate IPNS/KeyFS in this context; same for now
		case filesystem.IPNS, filesystem.KeyFS:
			if runtime.GOOS == "windows" && defaultConfig.Mounts.IPNS == nodeConf.Mounts.IPNS {
				targets = append(targets, platformMountRoot+"ipns")
				continue
			}
			targets = append(targets, nodeConf.Mounts.IPNS)
		case filesystem.Files:
			// TODO: config value default + platform portability
			targets = append(targets, platformMountRoot+"file")
			/* TODO overlayfs
			case Combined:
				// TODO: config value default + platform portability
				targets = append(targets, mount.SuggestedCombinedPath())
			*/
		default:
			return nil, fmt.Errorf("unexpected namespace: %s", system.String())
		}
	}
	return targets, nil
}

func combine(apis []manager.API, systems []filesystem.ID, targets []string) ([]manager.Request, error) {
	if tLen, nLen := len(targets), len(systems); tLen != nLen {
		return nil, fmt.Errorf("system and target count do not match([%d]system:%v|[%d]targets:%v)", nLen, systems, tLen, targets)
	}

	apiCount := len(apis)
	if apiCount == 1 { // special case, apply to all
		api := apis[0]
		targs := len(targets)
		apis = make([]manager.API, targs)
		for i := 0; i != targs; i++ {
			apis[i] = api
		}
	} else { // otherwise make sure everything aligns
		// TODO: for now restrict to one arg
		// later parse correctly
		return nil, fmt.Errorf("too many file system API's provided in request: %v", apis)
	}

	api := apis[0] // HACK: the pipeline needs to be better but isn't yet

	var requests []manager.Request
	for i, t := range targets {
		sysID := systems[i]

		request := manager.Request{
			Header:  manager.Header{ID: sysID},
			Request: host.Request{Target: t},
		}

		// process the target request into an API specific request
		switch api {
		default:
			return nil, fmt.Errorf("unexpected file system: %v", sysID)
		case manager.Plan9Protocol:
			var err error
			if request.Arguments, err = nineArgs(&request.Target, sysID); err != nil {
				return nil, err
			}
			request.API = manager.Plan9Protocol
		case manager.Fuse:
			request.Arguments = fuseArgs(&request.Target, sysID)
			request.API = manager.Fuse
		}

		requests = append(requests, request)
	}
	return requests, nil
}

// modifies the request target (if necessary for the platform/request)
// and may return a 9P specific parameter
// if the target is a file system path, to be mounted by us (as a client)
// the parameter string will specify a maddr for the provider to use (as the server)
// if the target is itself a listener address, it will be moved to the parameter string, and the target cleared
// (despite its name, the arity of `nineArgs` is 2, not 9)
func nineArgs(target *string, namespace filesystem.ID) ([]string, error) {
	// we allow templating unix domain socket maddrs, so check for those and expand them here
	if strings.HasPrefix(*target, "/unix") {
		listenerString, err := stabilizeUnixPath(*target)
		if err != nil {
			return nil, err
		}
		*target = listenerString
	}

	var listenerString string

	// if the target is a maddr, move it to the parameter string
	// to signify to the provider there is no mount target, and we just want the listener
	if _, err := multiaddr.NewMultiaddr(*target); err == nil {
		listenerString = *target
		*target = ""
	} else {
		// otherwise, provide a listening address for targets that are themselves, not-listeners
		// leaving the target string unmodified
		udsPath, err := stabilizeUnixPath(fmt.Sprintf("/unix/$IPFS_HOME/9p.%s.sock", namespace.String()))
		if err != nil {
			return nil, err
		}
		listenerString = udsPath
	}

	return []string{listenerString}, nil
}

// modifies the request target (if necessary for the platform/request)
// and may return an array of platform specific FUSE parameters, if needed
func fuseArgs(target *string, namespace filesystem.ID) []string {
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

		// basic Info
		/* TODO overlay
		if namespace == Combined {
			opts = "FileSystemName=IPFS,volname=IPFS"
		} else {
			opts = fmt.Sprintf("FileSystemName=%s,volname=%s", namespace.String(), namespace.String())
		}
		*/

		opts = fmt.Sprintf("FileSystemName=%s,volname=%s", namespace.String(), namespace.String())

		// set the owner to be the same as the process (`daemon`'s or `mount`'s depending on background/foreground)
		opts += ",uid=-1,gid=-1"
		args = append(args, "-o", opts)

		// convert UNC targets to WinFSP format
		if len(*target) > 2 && (*target)[:2] == `\\` {
			// NOTE: cgo-fuse/WinFSP UNC parameter uses single slash prefix, so we chop one off
			// the FUSE target uses `/`,
			// while the prefix parameter uses `\`
			// but otherwise they point to the same target
			//args = append(args, `--VolumePrefix`, (*target)[1:])
			//*target = ""
			//args = append(args, fmt.Sprintf(`--VolumePrefix=%s`, (*target)[1:]))

			//			modTarg := (*target)[1:]
			//args = append(args, fmt.Sprintf(`--VolumePrefix=%s`, modTarg))
			args = append(args, fmt.Sprintf(`--VolumePrefix=%s`, (*target)[1:]))
			//*target = filepath.ToSlash(modTarg)
			*target = ""
		}
		// otherwise target is another reference; leave it alone

	case "freebsd":
		/* TODO: overlay
		if namespace == Combined {
			opts = "fsname=IPFS,subtype=IPFS"
		} else {
			opts = fmt.Sprintf("fsname=%s,subtype=%s", namespace.String(), namespace.String())
		}
		*/

		opts = fmt.Sprintf("fsname=%s,subtype=%s", namespace.String(), namespace.String())

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
		/* TODO: overlay
		if namespace == filesystem.SystemIDCombined {
			// TODO: see if we can provide `volicon` via an IPFS path; or make the overlay provide one via `/.VolumeIcon.icns` on darwin
			opts = "fsname=IPFS,volname=IPFS"
		} else {
			opts = fmt.Sprintf("fsname=%s,volname=%s", namespace.String(), namespace.String())
		}
		*/
		opts = fmt.Sprintf("fsname=%s,volname=%s", namespace.String(), namespace.String())

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

	return args
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
