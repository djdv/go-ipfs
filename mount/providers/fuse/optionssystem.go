package fuse

import (
	mountcom "github.com/ipfs/go-ipfs/mount/utils/common"
	logging "github.com/ipfs/go-log"
)

type (
	InitSignal   chan error
	SystemOption interface{ apply(*systemSettings) }
)

type systemSettings struct {
	InitSignal                     // if provided, will be used to return a status from fs.Init()
	log        logging.EventLogger // if provided, will be used as the logger during operations

	// non-nilable
	//TODO:
	mountcom.ResourceLock // if provided, will replace the default lock used for operations
}

func parseSystemOptions(opts ...SystemOption) *systemSettings {
	settings := new(systemSettings)
	for _, opt := range opts {
		opt.apply(settings)
	}

	// non-nilable defaults
	if settings.ResourceLock == nil {
		settings.ResourceLock = mountcom.NewResourceLocker()
	}

	if settings.log == nil {
		settings.log = logging.Logger("fuse")
	}
	return settings
}

// WithInitSignal provides a channel that will receive a signal from within fs.Init()
func WithInitSignal(ic InitSignal) SystemOption {
	return initSignalOpt(ic)
}

// WithLog replaces the default logger
func WithLog(l logging.EventLogger) SystemOption {
	return logOpt(logOptContainer{l})
}

// WithResourceLock substitutes the default path locker used for operations by the fs
func WithResourceLock(rl mountcom.ResourceLock) SystemOption {
	return resourceLockOpt(resourceLockOptContainer{rl})
}

type (
	initSignalOpt            InitSignal
	logOpt                   logOptContainer
	logOptContainer          struct{ logging.EventLogger }
	resourceLockOpt          resourceLockOptContainer
	resourceLockOptContainer struct{ mountcom.ResourceLock }
)

func (ic initSignalOpt) apply(opts *systemSettings) {
	opts.InitSignal = InitSignal(ic)
}

func (lc logOpt) apply(opts *systemSettings) {
	opts.log = logging.EventLogger(lc.EventLogger)
}

func (rc resourceLockOpt) apply(opts *systemSettings) {
	opts.ResourceLock = mountcom.ResourceLock(rc.ResourceLock)
}

func maybeAppendLog(baseOpts []SystemOption, logName string) []SystemOption {
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
