// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package colors

import (
	"fmt"
	"io"
	"strings"
)

var dimColor = Color256(245) // a light gray

// An Atom is a segment of text with a single formatting style.
type Atom struct {
	S         string // the text
	Underline bool   // is it underlined?
	Bold      bool   // is it bold?
	Color     Code   // text color
}

// String returns the atom with the appropriate ANSI escape sequences.
func (a Atom) String() string {
	if !Enabled() {
		return a.S
	}

	var b strings.Builder
	b.WriteString(string(a.Color))
	if a.Underline {
		b.WriteString(string(Underline))
	}
	if a.Bold {
		b.WriteString(string(Bold))
	}
	b.WriteString(a.S)
	b.WriteString(string(Reset))
	return b.String()
}

// dimmed returns a copy of the atom with a dim gray color.
func (a Atom) dimmed() Atom {
	a.Color = dimColor
	return a
}

// Text represents a contiguous sequence of atoms.
type Text []Atom

// len returns the length of the printable characters in the text's constituent
// atoms. ANSI escape sequences are not counted as part of this length.
func (t Text) len() int {
	return len(t.raw())
}

// raw returns the raw underlying text without any ANSI escape sequences.
func (t Text) raw() string {
	var b strings.Builder
	for _, a := range t {
		b.WriteString(a.S)
	}
	return b.String()
}

// String returns the text with the appropriate ANSI escape sequences.
func (t Text) String() string {
	var b strings.Builder
	for _, a := range t {
		b.WriteString(a.String())
	}
	return b.String()
}

// dimmed returns a copy of the text with a dim gray color.
func (t Text) dimmed() Text {
	cloned := make(Text, len(t))
	for i, a := range t {
		cloned[i] = a.dimmed()
	}
	return Text(cloned)
}

// A Tabularizer produces pretty-printed tabularized text. Unlike
// tabwriter.Writer [1], Tabularizer properly handles text with ANSI escape
// codes. Here's what an example table looks like:
//
//	╭───────────────────────╮
//	│ CATS                  │
//	├────────┬─────┬────────┤
//	│ NAME   │ AGE │ COLOR  │
//	├────────┼─────┼────────┤
//	│ belle  │ 1y  │ tortie │
//	│ sidney │ 2y  │ calico │
//	│ dakota │ 8m  │ tuxedo │
//	╰────────┴─────┴────────╯
//
// The table format comes from duf [2].
//
// [1]: https://pkg.go.dev/text/tabwriter
// [2]: https://github.com/muesli/duf
type Tabularizer struct {
	w      io.Writer // where to write
	title  []Text    // table title
	rows   [][]Text  // buffered rows
	widths []int     // column widths
	dim    func(prev, row []string) []bool
}

// NewTabularizer returns a new tabularizer. The provided dim function
// determines which columns in a row, if any, are dimmed.
func NewTabularizer(w io.Writer, title []Text, dim func(prev, row []string) []bool) *Tabularizer {
	return &Tabularizer{w: w, title: title, dim: dim}
}

// Row buffers a new Row to be tabularized. The Row isn't written until Flush
// is called. Note that every Row reported to a tabularizer must be the same
// length. A value can be a text, atom, string, or fmt.Stringer.
func (t *Tabularizer) Row(values ...any) {
	// Coerce values into text.
	row := make([]Text, len(values))
	for i, v := range values {
		switch x := v.(type) {
		case Text:
			row[i] = x
		case Atom:
			row[i] = Text{x}
		case string:
			row[i] = Text{Atom{S: x}}
		case fmt.Stringer:
			row[i] = Text{Atom{S: x.String()}}
		default:
			panic(fmt.Errorf("unsupported value type %T", v))
		}
	}

	// Calculate column widths.
	if t.widths == nil {
		t.widths = make([]int, len(row))
	} else if len(t.widths) != len(row) {
		panic(fmt.Errorf("bad row size: got %d, want %d", len(row), len(t.widths)))
	}
	for i, text := range row {
		if text.len() > t.widths[i] {
			t.widths[i] = text.len()
		}
	}

	// Cache row.
	t.rows = append(t.rows, row)
}

// Flush writes all buffered rows. Flush should only be called once, after all
// rows have been written.
func (t *Tabularizer) Flush() {
	// writeRow writes a row with (l)eft, (m)iddle, and (r)ight separators. The
	// content between the separators is provided by the col function.
	writeRow := func(l, m, r string, col func(i, width int) string) {
		fmt.Fprint(t.w, l)
		for i, width := range t.widths {
			fmt.Fprint(t.w, col(i, width))
			if i != len(t.widths)-1 {
				fmt.Fprint(t.w, m)
			}
		}
		fmt.Fprintln(t.w, r)
	}

	width := 1 // initial vertical line
	for _, w := range t.widths {
		width += w + 3 // space before + space after + vertical line after
	}

	if t.title != nil {
		fmt.Fprintf(t.w, "╭%s╮\n", strings.Repeat("─", width-2))
		for _, title := range t.title {
			fmt.Fprintf(t.w, "│ %-*s │\n", width-4+len(title.String())-title.len(), title.String())
		}
		writeRow("├", "┬", "┤", func(i, w int) string {
			return strings.Repeat("─", w+2)
		})
	} else {
		writeRow("╭", "┬", "╮", func(i, w int) string {
			return strings.Repeat("─", w+2)
		})
	}
	writeRow("│", "│", "│", func(i, w int) string {
		text := t.rows[0][i]
		s := text.String()
		return fmt.Sprintf(" %-*s ", w+len(s)-text.len(), s)
	})
	writeRow("├", "┼", "┤", func(i, w int) string {
		return strings.Repeat("─", w+2)
	})
	for rid, row := range t.rows {
		if rid == 0 {
			continue
		}
		dim := make([]bool, len(t.widths))
		if Enabled() && rid > 1 {
			prev := make([]string, len(t.widths))
			for i, s := range t.rows[rid-1] {
				prev[i] = s.raw()
			}

			curr := make([]string, len(t.widths))
			for i, s := range row {
				curr[i] = s.raw()
			}

			dim = t.dim(prev, curr)
		}
		writeRow("│", "│", "│", func(i, w int) string {
			text := row[i]
			if dim[i] {
				text = text.dimmed()
			}
			s := text.String()
			return fmt.Sprintf(" %-*s ", w+len(s)-text.len(), s)
		})
	}
	writeRow("╰", "┴", "╯", func(i, w int) string {
		return strings.Repeat("─", w+2)
	})
}

// NoDim doesn't dim any columns.
func NoDim(prev, row []string) []bool {
	return make([]bool, len(row))
}

// PrefixDim dims the longest prefix of row that is identical to prev.
func PrefixDim(prev, row []string) []bool {
	cols := make([]bool, len(row))
	for i := 0; i < len(prev); i++ {
		if prev[i] == row[i] {
			cols[i] = true
		} else {
			return cols
		}
	}
	return cols
}

// FullDim dims any columns that are identical in prev.
func FullDim(prev, row []string) []bool {
	cols := make([]bool, len(row))
	for i := 0; i < len(prev); i++ {
		if prev[i] == row[i] {
			cols[i] = true
		}
	}
	return cols
}
