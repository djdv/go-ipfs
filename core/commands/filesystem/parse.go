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
		sharedOpts = []cmds.Option{
			cmds.StringOption(NamespaceKwd, cmdNamespaceDesc),
			cmds.StringOption(PathKwd, cmdPathDesc),
		}
	*/

	errParamNotProvided   = errors.New("parameter was not provided")
	errConfigNotProviding = errors.New("config does not provide requested value")
)

/* TODO: outdated; no longer true
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

	bindReq.Options = make(cmds.OptMap, len(sourceReq.Options)) // NOTE: we don't want to modify the source map
	for param, arg := range sourceReq.Options {
		switch {
		default: // don't copy options that don't apply to us
		case param == prefix: // don't copy the prefix itself
		case param == cmds.EncLong || param == cmds.EncShort:
			bindReq.Options[param] = arg // copy encoding option if present
		case strings.HasPrefix(param, prefix): // copy prefixed parameters, sans prefix
			bindReq.Options[strings.TrimPrefix(param, prefix+"-")] = arg // e.g. `superCmd --prefix-ABC=123` => `cmd -ABC=123`
		}
	}

	return &bindReq, true
}

func combine(apis []manager.API, systems []filesystem.ID, targets []string) ([]manager.Request, error) {
	// NOTE: if the argument lists provided don't have the same length
	// we assume the last argument of parameter X to repeat for parameter Y
	// e.g. {targ1, targ2, targ3} combined with {system1} results in
	// {targ1:system1, targ2:system1, targ3:system1}
	// this allows users on the command line to omit repeating arguments
	const errMissMatchFmt = "%s API and target count do not match([%d]apis:%v|[%d]targets:%v)"

	targetCount, systemCount := len(targets), len(systems)
	apiCount := len(apis)

	if systemCount < targetCount {
		systems = fillInNodeSystem(systems, systems[systemCount-1], targetCount-systemCount)
		systemCount = len(systems)
		if systemCount != targetCount {
			// TODO: should we panic here? this is more of an implementation error than a user error
			return nil, fmt.Errorf(errMissMatchFmt, "node",
				systemCount, systems, targetCount, targets)
		}
	}

	if apiCount < targetCount {
		apis = fillInHostSystem(apis, apis[apiCount-1], targetCount-apiCount)
		apiCount = len(apis)
		if apiCount != targetCount {
			return nil, fmt.Errorf(errMissMatchFmt, "host",
				apiCount, apis, targetCount, targets)
		}
	}

	var ( // all re-used in loop
		requests    = make([]manager.Request, 0, len(targets))
		hostRequest host.Request
		err         error
	)

	for i, target := range targets { // process target requests into API specific requests
		api := apis[i]
		sysID := systems[i]

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

func fillInHostSystem(systems []manager.API, system manager.API, count int) []manager.API {
	sLen := len(systems)

	filled := make([]manager.API, sLen+count)
	copy(filled, systems)
	tail := filled[sLen:]

	for i := range tail {
		tail[i] = system
	}

	return filled
}

func fillInNodeSystem(systems []filesystem.ID, system filesystem.ID, count int) []filesystem.ID {
	sLen := len(systems)

	filled := make([]filesystem.ID, sLen+count)
	copy(filled, systems)
	tail := filled[sLen:]

	for i := range tail {
		tail[i] = system
	}

	return filled
}

// TODO: update the parameter comments when they're finalized
func parseRequest(req *cmds.Request) ([]manager.Request, error) {
	const paramErrStr = "failed to get %s parameter arguments from request: %w"

	// --api=
	hostAPIs, err := parseHostAPIs(req)
	if err != nil {
		return nil, fmt.Errorf(paramErrStr, "host API", err)
	}

	// --subsystem=
	nodeAPIs, err := parseNodeAPIs(req)
	if err != nil {
		return nil, fmt.Errorf(paramErrStr, "node API", err)
	}

	// --target=
	targets, err := parseTargets(req)
	if err != nil {
		return nil, fmt.Errorf(paramErrStr, "target", err)
	}

	return combine(hostAPIs, nodeAPIs, targets)
}

func hasParam(req *cmds.Request, parameterName string) (string, bool) {
	subsystemString, found := req.Options[parameterName].(string)
	return subsystemString, found
}

func parseNodeAPIs(req *cmds.Request) ([]filesystem.ID, error) {
	// use args if provided
	if subsystemString, paramProvided := hasParam(req, subsystemKwd); paramProvided {
		return parseNodeAPIArg(subsystemString)
	}

	//  otherwise fallback to suggestions
	return parseNodeAPIArg(defaultNodeAPISetting)
}

func parseNodeAPIArg(arg string) ([]filesystem.ID, error) {
	idStrings, err := csv.NewReader(strings.NewReader(arg)).Read()
	if err != nil {
		return nil, err
	}

	typedIds := make([]filesystem.ID, 0, len(idStrings))
	for _, idString := range idStrings {
		typedSys, err := typeCastSystemArg(idString)
		if err != nil {
			return nil, err
		}
		typedIds = append(typedIds, typedSys)
	}

	return typedIds, nil
}

func parseHostAPIs(req *cmds.Request) ([]manager.API, error) {
	// use args if provided
	if apiString, paramProvided := hasParam(req, aPIKwd); paramProvided {
		return parseHostAPIArg(apiString)
	}
	return parseHostAPIArg(defaultHostAPISetting) //  otherwise fallback to suggestions
}

func parseHostAPIArg(arg string) ([]manager.API, error) {
	apiStrings, err := csv.NewReader(strings.NewReader(arg)).Read()
	if err != nil {
		return nil, err
	}

	apis := make([]manager.API, 0, len(apiStrings))
	for _, apiString := range apiStrings {
		typedAPI, err := typeCastAPIArg(apiString)
		if err != nil {
			return nil, err
		}
		apis = append(apis, typedAPI)
	}

	return apis, nil
}

func parseTargets(req *cmds.Request) ([]string, error) {
	// use args if provided
	if targetString, paramProvided := hasParam(req, TargetKwd); paramProvided {
		return parseTargetArg(targetString)
	}
	return defaultTargets, nil // otherwise fallback to suggestions
}

func parseTargetArg(arg string) ([]string, error) {
	targets, err := csv.NewReader(strings.NewReader(arg)).Read()
	if err != nil {
		return nil, err
	}
	return targets, nil
}

// TODO: move section to parseconfig.go maybe; split with parserequest.go
func parseEnvironment(env cmds.Environment) ([]manager.Request, error) {
	nodeConf, err := cmdenv.GetConfig(env)
	if err != nil {
		return nil, fmt.Errorf("failed to get config from node: %w", err)
	}

	hostAPIs, err := parseHostAPIConf(nodeConf)
	switch err {
	case nil:
	case errConfigNotProviding: // fall back to defaults if setting doesn't exist in config
		if hostAPIs, err = parseHostAPIArg(defaultHostAPISetting); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}

	nodeAPIs, err := parseNodeAPIConf(nodeConf)
	switch err {
	case nil:
	case errConfigNotProviding: // fall back to defaults if setting doesn't exist in config
		if nodeAPIs, err = parseNodeAPIArg(defaultNodeAPISetting); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}

	targets, err := parseTargetConfig(nodeConf, nodeAPIs)
	switch err {
	case nil:
	case errConfigNotProviding: // fall back to defaults if setting doesn't exist in config
		targets = defaultTargets
	default:
		return nil, err
	}

	return combine(hostAPIs, nodeAPIs, targets)
}

// TODO: config structure around this has not be defined
func parseHostAPIConf(nodeConf *config.Config) ([]manager.API, error) {
	return nil, errConfigNotProviding
}

// TODO: config structure around this has not be defined
func parseNodeAPIConf(nodeConf *config.Config) ([]filesystem.ID, error) {
	return nil, errConfigNotProviding
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
			return nil, fmt.Errorf("unexpected host system API: %s", system.String())
		}
	}

	if len(targets) == 0 {
		return nil, errConfigNotProviding
	}

	return targets, nil
}
