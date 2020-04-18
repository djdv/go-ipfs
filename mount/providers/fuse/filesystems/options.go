package fusecommon

type InitSignal chan error

type options struct {
	initSignal InitSignal // if provided, returns a status from fs.Init()
}

// WithInitSignal provides a channel that will receive an error from within fs.Init()
func WithInitSignal(ic chan error) Option {
	return initSignalOpt(ic)
}

type Option interface{ apply(*options) }

type (
	initSignalOpt InitSignal
)

func (ic initSignalOpt) apply(opts *options) {
	opts.initSignal = InitSignal(ic)
}
