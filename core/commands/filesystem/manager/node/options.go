package node

import (
	gopath "path"

	"github.com/ipfs/go-ipfs/filesystem"

	logging "github.com/ipfs/go-log"
	gomfs "github.com/ipfs/go-mfs"
)

const LogGroup = filesystem.LogGroup + "/fuse"

type settings struct {
	filesAPIRoot *gomfs.Root // required when mounting the FilesAPI namespace, otherwise nil-able
	log          logging.EventLogger
}

func parseOptions(opts ...Option) *settings {
	options := new(settings)
	for _, opt := range opts {
		opt.apply(options)
	}

	return options
}

type Option interface{ apply(*settings) }

type logOpt struct{ logging.EventLogger }

// WithLog replaces the default logger
func WithLog(l logging.EventLogger) Option { return logOpt{l} }
func (lo logOpt) apply(settings *settings) { settings.log = lo.EventLogger }

type filesOpt struct{ *gomfs.Root }

// WithFilesRoot provides an MFS root node to use for the FilesAPI namespace
func WithFilesAPIRoot(mroot *gomfs.Root) Option { return filesOpt{mroot} }
func (fr filesOpt) apply(settings *settings)    { settings.filesAPIRoot = fr.Root }

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
