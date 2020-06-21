package transformcommon

import "context"

type StreamSource interface {
	// generate a stream and start sending entries to the passed in channel (via a gorotuine)
	// closing the channel when you're done or canceled
	// in the event the stream cannot be generated, an error should be returned
	SendTo(context.Context, chan<- PartialEntry) error
}

type StreamBase struct {
	parentCtx    context.Context
	streamCancel context.CancelFunc
	err          error
	streamSource StreamSource
}

func NewStreamBase(ctx context.Context, cs StreamSource) *StreamBase {
	return &StreamBase{
		parentCtx:    ctx,
		streamSource: cs,
	}
}

func (ps *StreamBase) Open() (<-chan PartialEntry, error) {
	if ps.streamCancel != nil {
		return nil, ErrIsOpen
	}

	streamCtx, streamCancel := context.WithCancel(ps.parentCtx)

	listChan := make(chan PartialEntry, 1) // SendTo is responsible for this channel
	// it must close it when encountering an error or upon reaching the end of the stream

	if err := ps.streamSource.SendTo(streamCtx, listChan); err != nil {
		streamCancel()
		return nil, err
	}
	ps.streamCancel = streamCancel

	return listChan, nil
}

func (ps *StreamBase) Close() error {
	if ps.streamCancel == nil {
		return ErrNotOpen // double close is considered an error
	}

	ps.streamCancel()
	ps.streamCancel = nil

	ps.err = ErrNotOpen // invalidate future operations as we're closed
	return nil
}
