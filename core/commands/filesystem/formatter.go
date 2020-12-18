package fscmds

import (
	"context"
	"fmt"
	"io"
	"strings"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs-cmds/cli"
	"github.com/ipfs/go-ipfs/filesystem"
	"github.com/ipfs/go-ipfs/filesystem/manager"
	"github.com/ipfs/go-ipfs/filesystem/manager/errors"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/olekukonko/tablewriter"
)

//go:generate stringer -type=tableColumn -linecomment -output formatter_string.go
type tableColumn int

const (
	thHAPI    tableColumn = iota // Host API
	thNAPI                       // Node API
	thBinding                    // Binding
	thExtra                      // Options

	// NOTE: rows must align to this width
	tableWidth
)

// constructs the interface used to draw graphical tables to a writer
func newTableFormatter(writer io.Writer) *tablewriter.Table {
	tableHeader := []string{ // construct the header cells
		thHAPI.String(),
		thNAPI.String(),
		thBinding.String(),
		thExtra.String(),
	}

	table := tablewriter.NewWriter(writer) // construct the table renderer
	table.SetHeader(tableHeader)           // insert header cells

	hColors := make([]tablewriter.Colors, tableWidth) // apply styles to them
	for i := range hColors {
		hColors[i] = tablewriter.Colors{tablewriter.Bold}
	}
	table.SetHeaderColor(hColors...)

	// various frame decorations
	table.SetAutoFormatHeaders(false)
	table.SetBorder(false)
	table.SetColumnSeparator("│")
	table.SetCenterSeparator("┼")
	table.SetRowSeparator("─")

	/* NOTE: autowrap is a nice feature, but currently breaks a variety of things
	if the line is wrapped by the tablewriter
		) table.NumLines is inaccurate :^(
			) breaks cursor movement / redraw
		) colors apply to line 0, but wrapped lines are always plaintext
	we may need to fix this library or choose a different one, or just have simpler output
	*/
	table.SetAutoWrapText(false)

	return table
}

func responseAsTableRow(resp manager.Response) ([]string, []tablewriter.Colors) {
	row := make([]string, tableWidth)
	if maddr, err := multiaddr.NewMultiaddrBytes(resp.Request); err == nil {
		// retrieve row data from the multiaddr (if any)
		multiaddr.ForEach(multiaddr.Cast(resp.Request), func(com multiaddr.Component) bool {
			proto := com.Protocol()
			switch proto.Code {
			case int(filesystem.Fuse):
				row[thHAPI] = proto.Name
				row[thNAPI] = com.Value()

			case int(filesystem.Plan9Protocol):
				row[thHAPI] = proto.Name
				row[thNAPI] = com.Value()

				// XXX: quick 9P formatting hacks; make formal and break out of here
				_, tail := multiaddr.SplitFirst(maddr)       // strip fs header
				hopefullyNet, _ := multiaddr.SplitLast(tail) // strip path tail
				if addr, err := manet.ToNetAddr(hopefullyNet); err == nil {
					row[thExtra] = fmt.Sprintf("Listening on: %s://%s", addr.Network(), addr.String())
				} else {
					resp.Error = errors.MaybeWrap(resp.Error, err)
				}

			case int(filesystem.PathProtocol):
				row[thBinding] = com.Value()
			}
			return true
		})
	} else {
		resp.Error = errors.MaybeWrap(resp.Error, err)
	}

	// create the corresponding color values for the table's row
	// XXX: non-deuteranopia friendly colours
	rowColors := make([]tablewriter.Colors, tableWidth)
	for i := range rowColors {
		switch {
		case resp.Error == nil:
			rowColors[i] = tablewriter.Colors{tablewriter.FgGreenColor}
			continue

			// TODO: proper
		//case goerrors.Is(resp.Error, errUnwound):
		case strings.Contains(resp.Error.Error(), errUnwound.Error()): // XXX: no
			rowColors[i] = tablewriter.Colors{tablewriter.FgYellowColor}
		default:
			rowColors[i] = tablewriter.Colors{tablewriter.FgRedColor}
		}
		row[thExtra] = "/!\\ " + resp.Error.Error()
		// TODO: pointer to header table
		// change "Options" to "Options/Errors" dynamically
	}

	return row, rowColors
}

func drawResponse(graphics *tablewriter.Table, response manager.Response) {
	graphics.Rich(responseAsTableRow(response)) // adds the row to the table
	graphics.Render()                           // draws the entire table
}

func overdrawResponse(console io.Writer, scrollBack int, graphics *tablewriter.Table, response manager.Response) (err error) {
	const headerHeight = 2
	if scrollBack != 0 { // TODO: check/convert escapes code via some portability pkg
		_, err = console.Write([]byte( // pkg.AnsiToWindowsConsole(outString)
			fmt.Sprintf("\033[%dA\033[%dG",
				headerHeight+scrollBack, // move cursor up N lines
				0,                       // and go to the beginning of the line
			)))
	}
	drawResponse(graphics, response)
	return
}

// TODO: support `--enc`; if request encoding type is not "text", don't print extra messages or render the table
// just write responses directly to stdout
//
// formats responses for CLI/terminal displays
func mountFormatConsole(res cmds.Response, re cmds.ResponseEmitter) (err error) {
	// NOTE: We expect console requests to output to a terminal,
	// but that's not inherent. The user could request `--enc=not-text`, perhaps in a pipeline.
	// In which case we ignore our terminal rendering functions and just encode responses to stdout
	encType := cmds.GetEncoding(res.Request(), "")

	// if the request was made in the foreground (no existing daemon instance)
	// we'll block until we receive an interrupt (if no init errors are encountered)
	var (
		foregroundRequest               bool
		foregroundInstances             []manager.Response
		foregroundCtx, foregroundCancel = context.WithCancel(res.Request().Context)
	)
	if v, ok := res.Request().Root.Extra.GetValue("debug-foreground"); ok {
		foregroundRequest, _ = v.(bool)
	}

	defer func() {
		if err != nil {
			err = errors.MaybeWrap(err, re.CloseWithError(err))
			foregroundCancel()
		}
		if foregroundRequest {
			if err == nil && encType == cmds.Text {
				if err = re.Emit("Waiting in foreground, send interrupt to cancel\n"); err != nil {
					err = errors.MaybeWrap(err,
						fmt.Errorf("emitter encountered an error, exiting early: %w", err))
					return
				}
			}

			<-foregroundCtx.Done() // wait ...
			foregroundCancel()

			// close any instances we received from the emitter
			instanceChan := make(chan manager.Response, len(foregroundInstances))
			for _, instance := range foregroundInstances {
				instanceChan <- instance
			}
			close(instanceChan)
			for range closeAll(re, instanceChan) { // HACK: lol, no dude
			}
		}
	}()

	if encType != cmds.Text {
		err = mountFormatConsoleEncoded(res, re)
		return
	}

	var (
		console      = re.(cli.ResponseEmitter)
		scrollBack   int
		renderBuffer = console.Stdout()
		graphics     = newTableFormatter(renderBuffer)
		gotResponse  bool

		options          = res.Request().Options
		isListRequest, _ = options[listOptionKwd].(bool) // type-checked in pre-run
	)

	for {
		// show a preamble for attach requests
		// before we start waiting for attach responses
		if !gotResponse && !isListRequest {
			requests := res.Request().Arguments
			if len(requests) != 0 {
				if err = re.Emit(fmt.Sprintf("Attempting to bind to host system: %s...\n",
					strings.Join(requests, ", "),
				)); err != nil {
					return
				}
			}
		}

		// get request responses emitted from `Command.Run`
		// no order guaranteed, no length known
		var untypedResponse interface{}
		if untypedResponse, err = res.Next(); err != nil {
			if err == io.EOF { // no more responses
				err = nil
				break
			}
			return // some kind of emitter error was encountered
		}

		// TODO: I don't know if this is actually intentional or not
		// Emit will return a pointer to us if the command was executed remotely
		// and a non-pointer if local; regardless of the `Command.Type`'s type and what was actually passed to `Emit`
		// maybe a cmds-lib bug?
		var response manager.Response
		switch v := untypedResponse.(type) {
		case manager.Response:
			response = v
		case *manager.Response:
			response = *v
		case string: // TODO: reconsider the validity of this in regards to cmds-lib emitter expectations
			console.Stdout().Write([]byte(v)) // this might be fine, there may be a better way
			continue                          // to pass text messages between run and cli-postrun
		default:
			return fmt.Errorf("formatter received unexpected type+value: %#v", untypedResponse)
		}
		gotResponse = true // TODO: cleanup
		if foregroundRequest {
			if response.Error == nil {
				foregroundInstances = append(foregroundInstances, response)
			} else {
				foregroundCancel()
			}
		}

		// TODO: request options for overdraw and sort table
		// (re)render response to the console, as a formatted table
		//drawResponse(graphics, response)
		scrollBack = graphics.NumLines() // start drawing this many lines above the current line
		if err = overdrawResponse(renderBuffer, scrollBack, graphics, response); err != nil {
			return
		}

		// TODO: hardcoded async stdout printing ruins our output :^(
		// the logger gets a better error than the caller too...
		// https://github.com/bazil/fuse/blob/371fbbdaa8987b715bdd21d6adc4c9b20155f748/mount_linux.go#L98-L99

	}

	if isListRequest && !gotResponse {
		err = re.Emit("No active instances\n")
	}

	return
}

func mountFormatConsoleEncoded(res cmds.Response, re cmds.ResponseEmitter) (err error) {
	for {
		var v interface{}
		if v, err = res.Next(); err != nil {
			if err == io.EOF { // no more responses
				err = nil
				break
			}
			return // some kind of emitter error was encountered
		}
		// in this context, Emit will use the command's encoder map to encode the value
		// and send it to its writer (stdout in our case)
		if err = re.Emit(v); err != nil {
			return
		}
	}
	return
}

func closeAll(emitter cmds.ResponseEmitter, instances manager.Responses) errors.Stream {
	errors := make(chan error, len(instances))
	defer close(errors)
	// TODO: emitter could always fail; handle
	for instance := range instances {
		emitter.Emit(fmt.Sprintf("closing 📖 %v ...\n", instance))
		switch err := instance.Close(); err {
		case nil:
			emitter.Emit(fmt.Sprintf("closed 📗 %v\n", instance))
		default:
			emitter.Emit(fmt.Sprintf("failed to detach 📕 %v from host: %v\n", instance, err))
			errors <- err
		}
	}
	return errors
}
