// Package options is a WIP, standard set of host options that should be handled
package options

type (
	Option interface{ apply(*Settings) }
)

type Settings struct {
	LogPrefix string

	//foreground bool   // should the host operation block in the foreground until it exits or run in a background routine
}

func Parse(options ...Option) *Settings {
	settings := new(Settings)
	for _, opt := range options {
		opt.apply(settings)
	}

	return settings
}

type logPrefixOpt string

// WithLogPrefix adds a prefix to the log system's name/identifier
func WithLogPrefix(prefix string) Option             { return logPrefixOpt(prefix) }
func (prefix logPrefixOpt) apply(settings *Settings) { settings.LogPrefix = string(prefix) }

/*
type foregroundOpt bool

func (b foregroundOpt) apply(set *Settings) { set.foreground = bool(b) }
*/
