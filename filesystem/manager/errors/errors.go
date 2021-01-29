package errors

import (
	"context"
	"fmt"
	"sync"
)

type Stream = <-chan error

func Merge(errorStreams ...Stream) Stream {
	switch len(errorStreams) {
	case 0:
		empty := make(chan error)
		close(empty)
		return empty
	case 1:
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

func Splice(ctx context.Context, errorStreams <-chan Stream) Stream {
	streamPlex := make(chan error, len(errorStreams))

	var wg sync.WaitGroup
	sourceFrom := func(errors Stream) {
		defer wg.Done()
		for err := range errors {
			select {
			case streamPlex <- err:
			case <-ctx.Done():
				return
			}
		}
	}

	go func() {
		for errors := range errorStreams {
			wg.Add(1)
			go sourceFrom(errors)
		}
		wg.Wait()
		close(streamPlex)
	}()

	return streamPlex
}

func WaitForAny(ctx context.Context, errors ...Stream) (err error) {
	select {
	case err = <-Merge(errors...):
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
	combinedErrors := Merge(errors...)
	for {
		select {
		case e, ok := <-combinedErrors:
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
