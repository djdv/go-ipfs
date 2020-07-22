package fscmds

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io/ioutil"
	"runtime"
	"strings"

	cmds "github.com/ipfs/go-ipfs-cmds"
	config "github.com/ipfs/go-ipfs-config"
	"github.com/ipfs/go-ipfs/core/commands/cmdenv"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host"
	p9fsp "github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/9p"
	"github.com/ipfs/go-ipfs/core/commands/filesystem/manager/host/fuse"
	"github.com/ipfs/go-ipfs/filesystem"
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

// TranslateToBindRequest takes in a request for 1 command and translates it into a request for the base command.
// Copying parameters to the new request, without their prefix.
// returns true if the input request was translated into a bind request
// TODO: convert bool -> error; specify that a prefix was not found in the provided request
func TranslateToBindRequest(prefix string, sourceReq *cmds.Request) (*cmds.Request, bool) {
	if prefixFlag, _ := sourceReq.Options[prefix].(bool); !prefixFlag {
		return nil, false
	}

	bindReq := *sourceReq
	bindReq.Command = Mount

	delete(bindReq.Options, prefix) // delete the prefix option itself first

	for param, arg := range bindReq.Options {
		delete(bindReq.Options, param)        // delete the rest of the options
		if strings.HasPrefix(param, prefix) { // keeping (unencapsulated) copies of prefixed parameters
			// e.g. `superCmd --prefix-ABC=123` => `cmd -ABC=123`
			bindReq.Options[strings.TrimPrefix(param, prefix+"-")] = arg
		}
	}

	return &bindReq, true
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

	return combine(api, subsystems, targets)
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
	targetCount, systemCount := len(targets), len(systems)

	// TODO: we should do blank fill-ins here, not in the parserX functions
	// since there's ambiguity as to whether a value was provided or not
	// too much magic going on here

	// TODO: [HACK] this prevents the default system vector from causing issues on
	// single target requests like `ipfs mount --target=/somewhere`
	// but is magic and bad
	// also targets need to be the command argument, not flags
	if targetCount > systemCount {
		return nil, fmt.Errorf("targets ([%d]system:%v|[%d]targets:%v)", systemCount, systems, targetCount, targets)
	}

	apiCount := len(apis)
	if apiCount == 1 && targetCount != 1 { // special case, apply to all
		api := apis[0]
		apis = make([]manager.API, targetCount)
		for apiCount = 0; apiCount != targetCount; apiCount++ {
			apis[apiCount] = api
		}
	}
	if apiCount != targetCount {
		return nil, fmt.Errorf("host API and target count do not match([%d]apis:%v|[%d]targets:%v)", apiCount, apis, targetCount, targets)
	}

	var requests []manager.Request
	for i, target := range targets {
		api := apis[i]
		sysID := systems[i]

		// process the target request into an API specific request
		var (
			hostRequest host.Request
			err         error
		)
		switch api {
		default:
			err = fmt.Errorf("unexpected host API: %v", api)
		case manager.Plan9Protocol:
			hostRequest, err = p9fsp.ParseRequest(sysID, target)
		case manager.Fuse:
			hostRequest, err = fuse.ParseRequest(sysID, target)
		}
		if err != nil {
			return nil, err
		}

		requests = append(requests, manager.Request{
			Header:      manager.Header{API: api, ID: sysID},
			HostRequest: hostRequest,
		})
	}

	return requests, nil
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
