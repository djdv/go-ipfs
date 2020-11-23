package fscmds

import (
	"fmt"
	"io"
	"strings"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/go-ipfs/filesystem"
	"github.com/multiformats/go-multiaddr"
	"github.com/olekukonko/tablewriter"
)

//go:generate stringer -type=tableColumn -linecomment -output formatter_string.go
type tableColumn int

const (
	hAPI    tableColumn = iota // Host API
	nAPI                       // Node API
	binding                    // Binding
	options                    // Options

	// NOTE: rows must align to this width
	tableWidth
)

func newOutputTable(writer io.Writer) *tablewriter.Table {
	tableHeader := []string{ // construct the header cells
		hAPI.String(),
		nAPI.String(),
		binding.String(),
		options.String(),
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

	return table
}

func formatResponse(maddr multiaddr.Multiaddr) ([]string, []tablewriter.Colors) {
	row := make([]string, tableWidth)

	// retrieve row data from the multiaddr (if any)
	multiaddr.ForEach(maddr, func(com multiaddr.Component) bool {
		proto := com.Protocol()
		switch proto.Code {
		case int(filesystem.Fuse):
			row[hAPI] = proto.Name
			row[nAPI] = com.Value()
		case int(filesystem.PathProtocol):
			row[binding] = com.Value()
		}

		return true
	})

	// create the corresponding color values for the table's row
	rowColors := make([]tablewriter.Colors, tableWidth)
	for i := range rowColors {
		// format the response as green; signifying the request was honored
		rowColors[i] = tablewriter.Colors{tablewriter.FgGreenColor}
	}

	return row, rowColors
}

func runFormatCLI(res cmds.Response, re cmds.ResponseEmitter) (err error) {
	defer re.Close()
	if err = re.Emit("Binding node APIs to host system...\n"); err != nil {
		return
	}

	renderBuffer := new(strings.Builder)
	graphics := newOutputTable(renderBuffer)
	scrollBack := graphics.NumLines()

	for {
		var untypedResponse interface{}
		untypedResponse, err = res.Next()
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return
		}

		if scrollBack != 0 {
			// TODO: check/convert escapes code via some portability pkg
			// pkg.AnsiToWindowsConsole(outString)
			renderBuffer.WriteString(
				fmt.Sprintf("\033[%dA\033[%dG",
					2+scrollBack, // move cursor back up header lines + rows
					0,            // go to BoL
				))
		}

		graphics.Rich(formatResponse(untypedResponse.(multiaddr.Multiaddr)))
		graphics.Render()

		consoleOut := renderBuffer.String()
		scrollBack = graphics.NumLines()

		if err = re.Emit(consoleOut); err != nil {
			return
		}

		renderBuffer.Reset()
	}
}
