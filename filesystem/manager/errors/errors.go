package errors

import (
	"context"
	"fmt"
	"sync"
)

// TODO: scrutinize these functions
// they're small, but important for synchronizing

type Stream = <-chan error

func Combine(errorStreams ...Stream) Stream {

	switch len(errorStreams) {
	case 0: // this is most likely a logical error caller side, don't allow it
		panic("errors.Combine: no arguments provided; won't combine nothing")
	case 1: // TODO: justify 0 being disallowed and 1 being allowed; or rethink pipeline sink construction
		return errorStreams[0]
	}

	mergedStream := make(chan error)

	var wg sync.WaitGroup
	mergeFrom := func(errors Stream) {
		for err := range errors {
			mergedStream <- err
		}
		wg.Done()
	}

	wg.Add(len(errorStreams))
	for _, Errors := range errorStreams {
		go mergeFrom(Errors)
	}

	go func() { wg.Wait(); close(mergedStream) }()
	return mergedStream
}

func Merge(ctx context.Context, errorStreams <-chan Stream) Stream {
	var wg sync.WaitGroup
	mergedStream := make(chan error)
	mergeFrom := func(errors Stream) {
		defer wg.Done()
		for err := range errors {
			select {
			case mergedStream <- err:
			case <-ctx.Done():
				return
			}
		}
	}
	go func() {
		for errors := range errorStreams {
			wg.Add(1)
			mergeFrom(errors)
		}
		go func() { wg.Wait(); close(mergedStream) }()
	}()
	return mergedStream
}

func WaitForAny(ctx context.Context, errors ...Stream) (err error) {
	select {
	case err = <-Combine(errors...):
	case <-ctx.Done():
		err = ctx.Err()
	}
	return
}

func WaitFor(ctx context.Context, errors ...Stream) (err error) {
	maybeWrap := func(precedent, secondary error) error {
		if precedent == nil {
			return secondary
		} else if secondary != nil {
			return fmt.Errorf("%w - %s", precedent, secondary)
		}
		return nil
	}
	for {
		select {
		case e, ok := <-Combine(errors...):
			if !ok {
				return
			}
			err = maybeWrap(err, e)
		case <-ctx.Done():
			err = maybeWrap(err, ctx.Err())
			return
		}
	}
}
