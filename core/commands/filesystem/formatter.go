package fscmds

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/ipfs/go-ipfs-cmds/cli"
	"github.com/ipfs/go-ipfs/filesystem"
	"github.com/ipfs/go-ipfs/filesystem/manager"
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
				if hopefullyNet == nil {
					break
				}
				if addr, err := manet.ToNetAddr(hopefullyNet); err == nil {
					row[thExtra] = fmt.Sprintf("Listening on: %s://%s", addr.Network(), addr.String())
				} else {
					resp.Error = maybeWrap(resp.Error, err)
				}

			case int(filesystem.PathProtocol):
				row[thBinding] = com.Value()
			}
			return true
		})
	} else {
		resp.Error = maybeWrap(resp.Error, err)
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

// drawResponses renders the response stream to this console's output,
// and relays the stream as it's received.
func drawResponses(ctx context.Context, console cli.ResponseEmitter, responses manager.Responses) manager.Responses {
	var (
		relay = make(chan manager.Response)

		scrollBack   int
		renderBuffer = console.Stdout()
		graphics     = newTableFormatter(renderBuffer)
	)

	go func() {
		defer close(relay)
		for response := range responses { // (re)render response to the console, as a formatted table
			scrollBack = graphics.NumLines() // start drawing this many lines above the current line
			drawResponse(graphics, response)
			//overdrawResponse(renderBuffer, scrollBack, graphics, response)
			select {
			case relay <- response:
			case <-ctx.Done():
				return
			}
		}
	}()
	return relay
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

// TODO: move this somewhere else
func maybeWrap(precedent, secondary error) error {
	if precedent == nil {
		return secondary
	} else if secondary != nil {
		return fmt.Errorf("%w - %s", precedent, secondary)
	}
	return nil
}
