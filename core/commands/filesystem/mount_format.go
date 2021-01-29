package fscmds

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"time"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs-cmds/cli"
	"github.com/ipfs/go-ipfs/filesystem/manager"
	"github.com/ipfs/go-ipfs/filesystem/manager/errors"
)

func responsesToErrors(ctx context.Context, responses manager.Responses) errors.Stream {
	responseErrors := make(chan error, len(responses))
	go func() {
		defer close(responseErrors)
		var err error
		for response := range responses {
			if err = response.Error; err != nil {
				select {
				case responseErrors <- err:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return responseErrors
}

// emitResponses relays manager responses to an emitter,
// returning a copy of the stream, with emitter errors inserted into request values.
func emitResponses(ctx context.Context, emitter cmds.ResponseEmitter, responses manager.Responses) manager.Responses {
	relay := make(chan manager.Response, len(responses))
	go func() {
		defer close(relay)
		var emitErr error
		for response := range responses {
			if emitErr == nil { // emitter has an observer (formatter, API client, etc.)
				if emitErr = emitter.Emit(response); emitErr != nil { //try to emit to it
					// include emit errors in the responses
					response.Error = maybeWrap(response.Error, emitErr)
				}
			}
			select { // always relay
			case relay <- response:
			case <-ctx.Done():
				return
			}
		}
	}()
	return relay
}

//TODO: English
// cmdsResponseToManagerResponses transforms the cmds.Response stream into manager.Responses,
// if the emitter encounters an error, it is returned in a response.
func cmdsResponseToManagerResponses(ctx context.Context, response cmds.Response) manager.Responses {
	responses := make(chan manager.Response)
	go func() {
		defer close(responses)
		for {
			untypedResponse, err := response.Next()
			if err != nil {
				return // TODO: we might want to return emitter errors to the caller
				// we could use the Response.Error, and the caller just can know
				// that nil request, non-nil error == emitter error
				// or we return 2 streams, response, errors
			}

			// NOTE: Next is not guaranteed to return the exact type passed to `Emit`
			// local -> local responses are typically concrete copies directly returned from `Emit`,
			// with remote -> local responses typically being pointers returned from `Unmarshal`.
			var response manager.Response
			switch v := untypedResponse.(type) {
			case manager.Response:
				response = v
			case *manager.Response:
				response = *v
			default:
				// TODO: server sent garbage, what should we do here?
				continue // currently we ignore it
			}
			select {
			case responses <- response:
			case <-ctx.Done():
			}
		}
	}()
	return responses
}

type cmdsEmitFunc func(interface{}) error

func printXorRelay(encType cmds.EncodingType, re cmds.ResponseEmitter) (io.Writer, cmdsEmitFunc) {
	var (
		out                = ioutil.Discard
		emit               = re.Emit
		console, isConsole = re.(cli.ResponseEmitter)
		decorate           = isConsole && encType == cmds.Text
	)
	if decorate {
		out = console.Stdout()
		emit = func(interface{}) error { return nil }
	}
	return out, emit
}

func emitBind(ctx context.Context, emitter cmds.ResponseEmitter, response cmds.Response) (err error) {
	encType := cmds.GetEncoding(response.Request(), "")
	consoleOut, emit := printXorRelay(encType, emitter)
	decoWrite := func(s string) (err error) { _, err = consoleOut.Write([]byte(s)); return }

	stringArgs := response.Request().Arguments
	if len(stringArgs) != 0 {
		msg := fmt.Sprintf("Attempting to bind to host system: %s...\n", strings.Join(stringArgs, ", "))
		if err = decoWrite(msg); err != nil {
			return
		}
	}

	responses := cmdsResponseToManagerResponses(ctx, response)
	for fsResponse := range drawResponses(ctx, consoleOut, responses) {
		if fsResponse.Error != nil {
			err = fsResponse.Error // TODO: accumulate instead of lastError only
		}
		emit(fsResponse) // TODO: emitter errors too
	}

	return
}

func emitList(ctx context.Context, emitter cmds.ResponseEmitter, response cmds.Response) (err error) {
	encType := cmds.GetEncoding(response.Request(), "")
	consoleOut, emit := printXorRelay(encType, emitter)
	decoWrite := func(s string) (err error) { _, err = consoleOut.Write([]byte(s)); return }

	var gotResponse bool
	responses := cmdsResponseToManagerResponses(ctx, response)
	for fsResponse := range drawResponses(ctx, consoleOut, responses) {
		gotResponse = true
		emit(fsResponse) // TODO: emitter errors
	}

	if !gotResponse {
		err = decoWrite("No active instances\n")
	}

	return
}

func emitBindPostrun(request *cmds.Request, emitter cmds.ResponseEmitter, env cmds.Environment) (err error) {
	encType := cmds.GetEncoding(request, "")
	consoleOut, _ := printXorRelay(encType, emitter)
	decoWrite := func(s string) (err error) { _, err = consoleOut.Write([]byte(s)); return }

	if err := decoWrite("Waiting in foreground, send interrupt to cancel\n"); err != nil {
		return fmt.Errorf("emitter encountered an error, exiting early: %w", err)
	}
	<-request.Context.Done()

	// derive an `unmount` sub-request and execute it with an independent context
	var unmountRequest *cmds.Request
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel() // TODO: duration value is arbitrary; should be const somewhere too

	unmountRequest, err = cmds.NewRequest(ctx,
		[]string{UnmountParameter}, cmds.OptMap{cmds.EncLong: request.Options[cmds.EncLong], // inherit encoding
			unmountAllOptionKwd: true}, // detach all
		nil, nil,
		request.Root)
	if err == nil {
		err = cmds.NewExecutor(request.Root).Execute(unmountRequest, emitter, env)
	}
	return
}
