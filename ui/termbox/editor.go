package termbox

import (
	"xl/ui"

	"errors"
	"unicode/utf8"

	"github.com/nsf/termbox-go"
)

func (t *Termbox) enterEditorMode(config *editorConfig) (string, error) {
	defer func() {
		termbox.SetCursor(t.lastCursorX, t.lastCursorY)
	}()
	e := newEditor(config)
	for {
		event, err := t.ReadKey()
		if err != nil {
			return "", err
		}
		if keyEvent, ok := event.(ui.KeyEvent); ok {
			stop := e.OnKey(keyEvent)
			if stop {
				break
			}
		} else {
			return "", errors.New("unknown event")
		}
	}
	return e.Text(), nil
}

type ResizeEventDelegateInterface interface {
	OnResize(newLines int)
}

type editorConfig struct {
	X                   int
	Y                   int
	Width               int
	Height              int
	MaxRunes            int
	MaxLines            int
	ResizeEventDelegate ResizeEventDelegateInterface
	FgColor             int
	BgColor             int
	Value               string
}

type line struct {
	data []byte
	next *line
	prev *line
}

type cursor struct {
	line        *line
	offsetBytes int
	offsetRunes int
}

type window struct {
	topLine   *line
	firstRune int
}

type editor struct {
	config     *editorConfig
	cursor     cursor
	firstLine  *line
	lastLine   *line
	topLine    *line
	linesCount int
	window     window
}

func newEditor(config *editorConfig) *editor {
	l := line{
		// FIXME: assuming single line
		data: []byte(config.Value),
	}
	e := &editor{
		config: config,
		cursor: cursor{
			line:        &l,
			offsetRunes: utf8.RuneCountInString(config.Value),
			offsetBytes: len(config.Value),
		},
		window: window{
			topLine:   &l,
			firstRune: 0,
		},
		firstLine:  &l,
		lastLine:   &l,
		linesCount: 1,
	}
	e.redraw()
	return e
}

func (e *editor) OnKey(ev ui.KeyEvent) bool {
	switch ev.Key {
	case termbox.KeyCtrlF, termbox.KeyArrowRight:
		e.moveCursorForward()
	case termbox.KeyCtrlB, termbox.KeyArrowLeft:
		e.moveCursorBackward()
	case termbox.KeyCtrlN, termbox.KeyArrowDown:
		e.moveCursorNextLine()
	case termbox.KeyCtrlP, termbox.KeyArrowUp:
		e.moveCursorPrevLine()
	case termbox.KeyCtrlE, termbox.KeyEnd:
		//e.moveCursorEOL()
		//v.on_vcommand(vcommand_move_cursor_end_of_line, 0)
	case termbox.KeyCtrlA, termbox.KeyHome:
		//e.moveCursorBOL()
		//v.on_vcommand(vcommand_move_cursor_beginning_of_line, 0)
	case termbox.KeyCtrlV, termbox.KeyPgdn:
		//v.on_vcommand(vcommand_move_view_half_forward, 0)
	case termbox.KeyCtrlL:
		//v.on_vcommand(vcommand_recenter, 0)
	case termbox.KeyCtrlSlash:
		//v.on_vcommand(vcommand_undo, 0)
	case termbox.KeySpace:
		e.insertRune(' ')
	case termbox.KeyEnter, termbox.KeyCtrlJ:
		if e.config.MaxLines <= 1 {
			// exit editor when in single-line mode
			return true
		} else if e.linesCount < e.config.MaxLines {
			e.insertRune('\n')
		}
	case termbox.KeyBackspace, termbox.KeyBackspace2:
		if ev.Mod&termbox.ModAlt != 0 {
			//e.deleteWordBackward()
		} else {
			e.deleteRuneBackward()
		}
	case termbox.KeyDelete, termbox.KeyCtrlD:
		e.deleteRune()
	case termbox.KeyCtrlK:
		//v.on_vcommand(vcommand_kill_line, 0)
	case termbox.KeyPgup:
		//v.on_vcommand(vcommand_move_view_half_backward, 0)
	case termbox.KeyTab:
		e.insertRune('\t')
	case termbox.KeyCtrlSpace:
		//if ev.Ch == 0 {
		//	v.set_mark()
		//}
	case termbox.KeyCtrlW:
		//v.on_vcommand(vcommand_kill_region, 0)
	case termbox.KeyCtrlY:
		//v.on_vcommand(vcommand_yank, 0)
	case termbox.KeyEsc:
		// edit editor, discard changes
		return true
	}

	if ev.Ch != 0 {
		e.insertRune(ev.Ch)
	}

	e.redraw()

	return false
}

func (e *editor) Text() string {
	// FIXME: assuming single line
	return string(e.firstLine.data)
}

// insertRune inserts a rune 'r' at the current cursor position,
// advance cursor one character forward.
func (e *editor) insertRune(r rune) {
	if r == '\n' {
		e.insertLine()
		e.adjustWindow()
		return
	}
	data := make([]byte, utf8.UTFMax)
	l := utf8.EncodeRune(data, r)
	e.cursor.line.data = insertBytes(e.cursor.line.data, e.cursor.offsetBytes, data[:l])
	e.cursor.offsetBytes += l
	e.cursor.offsetRunes++
	e.adjustWindow()
}

// deleteRune deleted a rune under cursor. If cursor at end of line,
// connects next line to the end of current line.
func (e *editor) deleteRune() {
	line := e.cursor.line
	if e.eol() {
		if e.eof() {
			return
		}
		// If cursor at end of line, connect next line to the end of current line.
		line.data = append(line.data, line.next.data...)
		if line.next != nil {
			line.next.prev = line
			line.next = line.next.next
		}
		e.linesCount--
		e.adjustWindow()
		return
	}
	_, l := utf8.DecodeRune(line.data[e.cursor.offsetBytes:])
	e.deleteBytesAtCursor(l)
	e.adjustWindow()
}

// deleteRuneBackward deleted previous rune.
func (e *editor) deleteRuneBackward() {
	line := e.cursor.line
	if e.bol() {
		if e.bof() {
			return
		}
		// If cursor at beginning of line, connects current line to the end of previous.
		e.cursor.offsetBytes = len(line.prev.data)
		e.cursor.offsetRunes = utf8.RuneCountInString(string(line.prev.data))
		line.prev.data = append(line.prev.data, line.data...)
		line.prev.next = line.next
		if line.next != nil {
			line.next.prev = line.prev
		}
		e.cursor.line = line.prev
		e.linesCount--
		e.adjustWindow()
		return
	}
	_, l := utf8.DecodeLastRune(line.data[:e.cursor.offsetBytes])
	e.cursor.offsetBytes -= l
	e.cursor.offsetRunes--
	e.deleteBytesAtCursor(l)
	e.adjustWindow()
}

func (e *editor) moveCursorForward() {
	line := e.cursor.line
	if e.eol() {
		if line.next == nil {
			return
		}
		e.cursor.line = line.next
		e.cursor.offsetBytes = 0
		e.cursor.offsetRunes = 0
		e.adjustWindow()
		return
	}
	_, l := utf8.DecodeRune(line.data[e.cursor.offsetBytes:])
	e.cursor.offsetBytes += l
	e.cursor.offsetRunes++
	e.adjustWindow()
}

func (e *editor) moveCursorBackward() {
	line := e.cursor.line
	if e.bol() {
		if line.prev == nil {
			return
		}
		e.cursor.line = line.prev
		e.cursor.offsetBytes = len(line.prev.data)
		e.cursor.offsetRunes = utf8.RuneCountInString(string(line.prev.data))
		e.adjustWindow()
		return
	}
	_, l := utf8.DecodeLastRune(line.data[:e.cursor.offsetBytes])
	e.cursor.offsetBytes -= l
	e.cursor.offsetRunes--
	e.adjustWindow()
}

func (e *editor) moveCursorNextLine() {
	line := e.cursor.line
	if line.next == nil {
		return
	}
	runesLen := utf8.RuneCountInString(string(line.next.data))
	if runesLen < e.cursor.offsetRunes {
		e.cursor.offsetRunes = runesLen
	}
	runes := []rune(string(line.next.data))
	e.cursor.offsetBytes = len(string(runes[:e.cursor.offsetRunes]))
	e.cursor.line = line.next
	e.adjustWindow()
}

func (e *editor) moveCursorPrevLine() {
	line := e.cursor.line
	if line.prev == nil {
		return
	}
	runesLen := utf8.RuneCountInString(string(line.prev.data))
	if runesLen < e.cursor.offsetRunes {
		e.cursor.offsetRunes = runesLen
	}
	runes := []rune(string(line.prev.data))
	e.cursor.offsetBytes = len(string(runes[:e.cursor.offsetRunes]))
	e.cursor.line = line.prev
	e.adjustWindow()
}

// bol is true if cursor at beginning of line
func (e *editor) bol() bool {
	return e.cursor.offsetBytes == 0
}

// bof is true if cursor at beginning of text
func (e *editor) bof() bool {
	return e.bol() && e.cursor.line.prev == nil
}

// bol is true if cursor at end of line
func (e *editor) eol() bool {
	return e.cursor.offsetBytes == len(e.cursor.line.data)
}

// bof is true if cursor at end of text
func (e *editor) eof() bool {
	return e.eol() && e.cursor.line.next == nil
}

func (e *editor) deleteBytesAtCursor(n int) {
	line := e.cursor.line
	// delete a chunk of data
	copy(line.data[e.cursor.offsetBytes:], line.data[e.cursor.offsetBytes+n:])
	line.data = line.data[:len(line.data)-n]
}

func (e *editor) insertLine() {
	current := e.cursor.line
	newLine := line{
		prev: current,
		next: current.next,
		data: cloneBytes(current.data[e.cursor.offsetBytes:]),
	}
	current.data = current.data[:e.cursor.offsetBytes]

	// refresh links
	current.next = &newLine
	if newLine.next != nil {
		newLine.next.prev = &newLine
	}

	// move cursor
	e.cursor.line = &newLine
	e.cursor.offsetRunes = 0
	e.cursor.offsetBytes = 0

	e.linesCount++
}

func (e *editor) redraw() {
	y := e.config.Y
	line := e.window.topLine
	for y-e.config.Y < e.config.Height {
		text := ""
		if line != nil && e.window.firstRune < len(string(line.data)) {
			text = string(line.data)[e.window.firstRune:]
		}
		drawCell(e.config.X, y, e.config.Width, 1, text, e.config.FgColor, e.config.BgColor)
		if line != nil {
			if line == e.cursor.line {
				termbox.SetCursor(e.config.X+e.cursor.offsetRunes-e.window.firstRune, y)
			}
			// advance to next line
			line = line.next
		}
		y++
	}
	_ = termbox.Flush()
}

func (e *editor) adjustWindow() {
	if e.window.firstRune < e.cursor.offsetRunes-(e.config.Width-1) {
		e.window.firstRune = e.cursor.offsetRunes - (e.config.Width - 1)
	} else if e.window.firstRune > e.cursor.offsetRunes {
		e.window.firstRune = e.cursor.offsetRunes
	}
	//if e.cursor.lineNum
}

func cloneBytes(s []byte) []byte {
	c := make([]byte, len(s))
	copy(c, s)
	return c
}

func insertBytes(s []byte, offset int, data []byte) []byte {
	n := len(s) + len(data)
	s = growByteSlice(s, n)
	s = s[:n]
	copy(s[offset+len(data):], s[offset:])
	copy(s[offset:], data)
	return s
}

func growByteSlice(s []byte, desiredCap int) []byte {
	if cap(s) < desiredCap {
		ns := make([]byte, len(s), desiredCap)
		copy(ns, s)
		return ns
	}
	return s
}
