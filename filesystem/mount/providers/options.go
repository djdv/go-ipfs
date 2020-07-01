package providercommon

import (
	"runtime"

	logging "github.com/ipfs/go-log"
	gomfs "github.com/ipfs/go-mfs"
)

// NOTE: [b7952c54-1614-45ea-a042-7cfae90c5361] cgofuse only supports ReaddirPlus on Windows
// if this ever changes (bumps libfuse from 2.8 -> 3.X+), add platform support here (and to any other tags with this UUID)
// TODO: this would be best in the fuselib itself; make a patch upstream
// it's only here because we can't put it in fusecommon because of a dependency cycle
const CanReaddirPlus bool = runtime.GOOS == "windows"

type Settings struct {
	ResourceLock ResourceLock // if provided, will replace the default lock used for operations
	FilesAPIRoot *gomfs.Root  // required when mounting the FilesAPI namespace, otherwise nil-able
	Log          logging.EventLogger
}

func ParseOptions(opts ...Option) *Settings {
	options := new(Settings)
	for _, opt := range opts {
		opt.apply(options)
	}

	return options
}

// WithLog replaces the default logger
func WithLog(l logging.EventLogger) Option {
	return logOpt(logOptContainer{l})
}

// WithFilesRoot provides an MFS root node to use for the FilesAPI namespace
func WithFilesAPIRoot(mroot *gomfs.Root) Option {
	return mfsOpt(mfsOptContainer{mroot})
}

// WithResourceLock substitutes the default path locker used for operations by the fs
func WithResourceLock(rl ResourceLock) Option {
	return resourceLockOpt(resourceLockOptContainer{rl})
}

type Option interface{ apply(*Settings) }

type (
	logOpt                   logOptContainer
	logOptContainer          struct{ logging.EventLogger }
	resourceLockOpt          resourceLockOptContainer
	resourceLockOptContainer struct{ ResourceLock }
	mfsOpt                   mfsOptContainer
	mfsOptContainer          struct{ *gomfs.Root }
)

func (lc logOpt) apply(settings *Settings) {
	settings.Log = logging.EventLogger(lc.EventLogger)
}

func (rc resourceLockOpt) apply(settings *Settings) {
	settings.ResourceLock = ResourceLock(rc.ResourceLock)
}

func (rc mfsOpt) apply(settings *Settings) {
	settings.FilesAPIRoot = (*gomfs.Root)(rc.Root)
}

func MaybeAppendLog(baseOpts []Option, logName string) []Option {
	var logWasProvided bool
	for _, opt := range baseOpts {
		if _, logWasProvided = opt.(logOpt); logWasProvided {
			break
		}
	}

	if !logWasProvided {
		baseOpts = append(baseOpts, WithLog(logging.Logger(logName)))
	}
	return baseOpts
}
