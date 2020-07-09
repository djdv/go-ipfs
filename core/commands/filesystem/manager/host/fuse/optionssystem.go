package fuse

import (
	"github.com/ipfs/go-ipfs/filesystem"
	logging "github.com/ipfs/go-log"
	gomfs "github.com/ipfs/go-mfs"
)

const LogGroup = filesystem.LogGroup + "/fuse"

type (
	InitSignal chan error
	Option     interface{ apply(*systemSettings) }
)

type systemSettings struct {
	InitSignal                       // if provided, will be used to return a status from fuseInterface.Init()
	log          logging.EventLogger // if provided, will be used as the logger during operations
	foreground   bool                // should the provider block in the foreground until it exits or run in a background routine
	filesAPIRoot *gomfs.Root         // required when mounting the FilesAPI namespace, otherwise nil-able
}

func parseOptions(options ...Option) *systemSettings {
	settings := new(systemSettings)
	for _, opt := range maybeAppendLog(options, LogGroup) {
		opt.apply(settings)
	}

	return settings
}

type initSignalOpt InitSignal

// WithInitSignal provides a channel that will receive a signal from within fuseInterface.Init()
func WithInitSignal(ic InitSignal) Option               { return initSignalOpt(ic) }
func (io initSignalOpt) apply(settings *systemSettings) { settings.InitSignal = InitSignal(io) }

type filesAPIOpt struct{ *gomfs.Root }

// WithFilesRoot provides an MFS root node to use for the FilesAPI namespace
func WithFilesAPIRoot(mroot *gomfs.Root) Option  { return filesAPIOpt{mroot} }
func (rc filesAPIOpt) apply(set *systemSettings) { set.filesAPIRoot = rc.Root }

type logOpt struct{ logging.EventLogger }

// WithLog replaces the default logger
func WithLog(l logging.EventLogger) Option       { return logOpt{l} }
func (lo logOpt) apply(settings *systemSettings) { settings.log = lo.EventLogger }
func maybeAppendLog(baseOpts []Option, logName string) []Option {
	var logWasProvided bool
	for _, opt := range baseOpts {
		if _, logWasProvided = opt.(logOpt); logWasProvided {
			break
		}
	}

	if !logWasProvided {
		baseOpts = append(baseOpts, WithLog(logging.Logger(LogGroup+"/"+logName)))
	}
	return baseOpts
}

type foregroundOpt bool

func (b foregroundOpt) apply(set *systemSettings) { set.foreground = bool(b) }
