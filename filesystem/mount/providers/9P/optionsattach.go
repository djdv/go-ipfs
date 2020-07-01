package p9fsp

import (
	"github.com/ipfs/go-ipfs/filesystem/mount"
	provcom "github.com/ipfs/go-ipfs/filesystem/mount/providers"
	logging "github.com/ipfs/go-log"
)

const LogGroup = mount.LogGroup + "/9P"

type AttachOption interface{ apply(*attachSettings) }

type attachSettings struct {
	log logging.EventLogger

	// non-nilable
	// TODO:
	provcom.ResourceLock // if provided, will replace the default lock used for operation
}

func parseAttachOptions(options ...AttachOption) *attachSettings {
	settings := new(attachSettings)
	for _, opt := range options {
		opt.apply(settings)
	}

	// non-nilable defaults
	if settings.ResourceLock == nil {
		settings.ResourceLock = provcom.NewResourceLocker()
	}

	if settings.log == nil {
		settings.log = logging.Logger(LogGroup)
	}

	return settings
}

// WithLog replaces the default logger
func WithLog(l logging.EventLogger) AttachOption {
	return logOpt(logOptContainer{l})
}

type (
	logOpt          logOptContainer
	logOptContainer struct{ logging.EventLogger }
)

func (lc logOpt) apply(settings *attachSettings) {
	settings.log = logging.EventLogger(lc.EventLogger)
}

func maybeAppendLog(baseOpts []AttachOption, logName string) []AttachOption {
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
