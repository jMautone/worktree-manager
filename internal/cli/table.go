package cli

import (
	"bytes"
	"io"
	"strings"
	"unicode/utf8"
)

// SGR parameters for the few styles wt uses.
const (
	styleBold    = "1"
	styleDim     = "2"
	styleCurrent = "1;32"
	styleMain    = "34"
	styleBranch  = "36"
	styleWarn    = "33"
	styleError   = "31"
)

// cell is one table cell: its text and an optional style.
type cell struct {
	text  string
	style string
}

// renderTable writes rows as aligned columns separated by two spaces.
//
// text/tabwriter measures bytes, so ANSI sequences break alignment as soon as
// one row has color and another does not. Here widths are computed on the
// plain text, and color wraps the text after the padding has been decided.
// Trailing empty cells are not padded, so no line ends in spaces.
func renderTable(w io.Writer, rows [][]cell, color bool) error {
	var widths []int
	for _, row := range rows {
		for i, c := range row {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], utf8.RuneCountInString(c.text))
		}
	}
	var buf bytes.Buffer
	for _, row := range rows {
		last := len(row) - 1
		for last > 0 && row[last].text == "" {
			last--
		}
		for i, c := range row[:last+1] {
			if i > 0 {
				buf.WriteString("  ")
			}
			if color && c.style != "" && c.text != "" {
				buf.WriteString("\x1b[" + c.style + "m" + c.text + "\x1b[0m")
			} else {
				buf.WriteString(c.text)
			}
			if i < last {
				buf.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c.text)))
			}
		}
		buf.WriteByte('\n')
	}
	_, err := w.Write(buf.Bytes())
	return err
}
