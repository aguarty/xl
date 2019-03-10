package app

import (
	"xl/document/sheet"
	"xl/ui"

	"fmt"
	"strings"
)

const colSizeIncrementStep = 6

// processCommand do the job associated with the command.
// If no such command found, shows the error in status line.
func (a *App) processCommand(c string) bool {
	c, args := parseArgs(c)
	switch c {
	case "q", "quit":
		return true
	case "w", "write":
		a.cmdWrite(arg1(args))
	case "wider":
		a.cmdResizeColumn(1)
	case "narrower":
		a.cmdResizeColumn(-1)
	case "as", "appendSheet":
		a.cmdNewSheet(arg1(args))
	case "ns", "nextSheet":
		a.cmdNextSheet()
	default:
		a.output.SetStatus(fmt.Sprintf("unknown command %s", c), ui.StatusFlagError)
	}
	return false
}

// cmdResizeColumn resizes column under cursor so its width becomes given N pixels.
func (a *App) cmdResizeColumn(n int) {
	col := a.doc.CurrentSheet.Cursor.X
	size := a.doc.CurrentSheet.ColSize(col)
	a.doc.CurrentSheet.SetColSize(col, size+n*colSizeIncrementStep)
	a.output.SetDirty(ui.DirtyHRuler | ui.DirtyGrid)
}

// cmdWrite saves document to file.
func (a *App) cmdWrite(filename string) {
	var err error
	if filename != "" {
		err = a.WriteAs(filename)
	} else {
		err = a.Write()
	}
	if err != nil {
		a.ShowError(err)
	}
}

// cmdNewList creates a new sheet.
func (a *App) cmdNewSheet(title string) {
	// FIXME: title must be unique
	if title == "" {
		title = fmt.Sprintf("Sheet %d", len(a.doc.Sheets)+1)
	}
	s := sheet.New(title)
	a.doc.Sheets = append(a.doc.Sheets, s)
	a.output.SetDirty(ui.DirtyStatusLine)
}

// cmdNextSheet switches the current sheet to next one.
// If current sheet is the last one, it switches to first.
func (a *App) cmdNextSheet() {
	a.doc.CurrentSheetN++
	if a.doc.CurrentSheetN >= len(a.doc.Sheets) {
		a.doc.CurrentSheetN = 0
	}
	a.doc.CurrentSheet = a.doc.Sheets[a.doc.CurrentSheetN]
	a.output.SetDirty(ui.DirtyStatusLine | ui.DirtyGrid | ui.DirtyFormulaLine)
}

// arg1 returns first argument or empty string.
func arg1(args []string) string {
	return argN(args, 1)
}

// argN returns Nth argument or empty string.
func argN(args []string, n int) string {
	if len(args) >= n {
		return args[n-1]
	}
	return ""
}

// parseArgs splits raw command line into command itself and list of command arguments.
// TODO: arguments can possibly be wrapped in quotes
func parseArgs(cmd string) (string, []string) {
	// FIXME: naive implementation
	c := strings.Split(cmd, " ")
	return c[0], c[1:]
}