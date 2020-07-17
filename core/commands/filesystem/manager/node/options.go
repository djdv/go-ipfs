package node

import (
	"github.com/ipfs/go-ipfs/filesystem"

	logging "github.com/ipfs/go-log"
)

const LogGroup = filesystem.LogGroup

type systemSettings struct {
	log logging.EventLogger
}

func parseOptions(options ...Option) *systemSettings {
	settings := new(systemSettings)
	for _, opt := range options {
		opt.apply(settings)
	}

	if settings.log == nil {
		settings.log = logging.Logger(LogGroup)
	}

	return settings
}

type Option interface{ apply(*systemSettings) }

type logOpt struct{ logging.EventLogger }

// WithLog replaces the default logger
func WithLog(l logging.EventLogger) Option       { return logOpt{l} }
func (lo logOpt) apply(settings *systemSettings) { settings.log = lo.EventLogger }
