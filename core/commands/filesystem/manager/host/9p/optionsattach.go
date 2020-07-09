package p9fsp

import (
	gopath "path"

	"github.com/ipfs/go-ipfs/filesystem"
	logging "github.com/ipfs/go-log"
)

const LogGroup = filesystem.LogGroup + "/9p"

type Option interface{ apply(*settings) }

type settings struct {
	log logging.EventLogger
}

func parseAttachOptions(options ...Option) *settings {
	settings := new(settings)
	for _, opt := range options {
		opt.apply(settings)
	}

	// non-nilable defaults
	if settings.log == nil {
		settings.log = logging.Logger(LogGroup)
	}

	return settings
}

type logOpt struct{ logging.EventLogger }

// WithLog replaces the default logger
func WithLog(l logging.EventLogger) Option { return logOpt{l} }
func (lo logOpt) apply(settings *settings) { settings.log = lo.EventLogger }
func maybeAppendLog(baseOpts []Option, logName string) []Option {
	var logWasProvided bool
	for _, opt := range baseOpts {
		if _, logWasProvided = opt.(logOpt); logWasProvided {
			break
		}
	}

	if !logWasProvided {
		baseOpts = append(baseOpts, WithLog(logging.Logger(gopath.Join(LogGroup, logName))))
	}
	return baseOpts
}
