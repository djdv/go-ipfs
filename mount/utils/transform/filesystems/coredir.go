package transformcommon

import "context"

// TODO: is there a better name for this?
// method name is also bad; try again later
type CoreStreamSource interface {
	// generate a stream and start sending entries to the passed in channel (via a gorotuine)
	// closing the channel when you're done or canceled
	// in the event the stream cannot be generated, an error should be returned
	SendTo(context.Context, chan<- PartialEntry) error
}

// TODO: is there a better name for this?
type CoreStreamBase struct {
	openCtx      context.Context
	streamCancel context.CancelFunc
	err          error
	streamSource CoreStreamSource
}

func NewCoreStreamBase(ctx context.Context, cs CoreStreamSource) *CoreStreamBase {
	return &CoreStreamBase{
		openCtx:      ctx,
		streamSource: cs,
	}
}

func (ps *CoreStreamBase) Open() (<-chan PartialEntry, error) {
	if ps.streamCancel != nil {
		return nil, ErrIsOpen
	}

	streamCtx, streamCancel := context.WithCancel(ps.openCtx)

	listChan := make(chan PartialEntry, 1) // closed by SendTo

	if err := ps.streamSource.SendTo(streamCtx, listChan); err != nil {
		streamCancel()
	}
	ps.streamCancel = streamCancel

	return listChan, nil
}

func (ps *CoreStreamBase) Close() error {
	if ps.streamCancel == nil {
		return ErrNotOpen // double close is considered an error
	}

	ps.streamCancel()
	ps.streamCancel = nil

	ps.err = ErrNotOpen // invalidate future operations as we're closed
	return nil
}
