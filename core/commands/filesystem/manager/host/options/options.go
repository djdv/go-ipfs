// Package options is a WIP, standard set of host options that should be handled
package options

type (
	InitSignal chan error
	Option     interface{ apply(*Settings) }
)

type Settings struct {
	// TODO: this needs to be a pair of signals
	// {init/stop}
	InitSignal // if provided, will be used to return a status from nodeBinding.Init()
	LogPrefix  string

	//foreground bool   // should the host operation block in the foreground until it exits or run in a background routine
}

func Parse(options ...Option) *Settings {
	settings := new(Settings)
	for _, opt := range options {
		opt.apply(settings)
	}

	return settings
}

type initSignalOpt InitSignal

// WithInitSignal provides a channel that will receive a signal from within nodeBinding.Init()
func WithInitSignal(ic InitSignal) Option         { return initSignalOpt(ic) }
func (io initSignalOpt) apply(settings *Settings) { settings.InitSignal = InitSignal(io) }

type logPrefixOpt string

// WithLogPrefix adds a prefix to the log system's name/identifier
func WithLogPrefix(prefix string) Option             { return logPrefixOpt(prefix) }
func (prefix logPrefixOpt) apply(settings *Settings) { settings.LogPrefix = string(prefix) }

/*
type foregroundOpt bool

func (b foregroundOpt) apply(set *Settings) { set.foreground = bool(b) }
*/
