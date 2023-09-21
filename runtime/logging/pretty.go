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

package logging

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/protos"
)

var (
	dimColor   = colors.Color256(245) // dimmed text color (a light gray)
	errorColor = colors.Color256(9)   // error color (a light red)
)

// PrettyPrinter pretty prints log entries. You can safely use a PrettyPrinter
// from multiple goroutines.
type PrettyPrinter struct {
	colorize func(colors.Code, string) string // colors the provided string

	mu               sync.Mutex      // guards the following fields
	b                strings.Builder // used to format entries
	prevTime         time.Time       // Last line timestamp
	componentPadding int             // component padding
	sourcePadding    int             // file:line padding
	attrs            [][2]string     // Temporary storage
}

// NewPrettyPrinter returns a new PrettyPrinter. If color is true, the pretty
// printer colorizes its output using ANSII escape codes.
func NewPrettyPrinter(color bool) *PrettyPrinter {
	pp := &PrettyPrinter{
		colorize:         func(_ colors.Code, s string) string { return s },
		componentPadding: 20,
		sourcePadding:    20,
	}
	if color {
		pp.colorize = func(code colors.Code, s string) string {
			return fmt.Sprintf("%s%s%s", code, s, colors.Reset)
		}
	}
	return pp
}

// Format formats a log entry as a single line of human-readable text. Here are
// some examples of what pretty printed log entries look like:
//
//	I0921 10:07:31.733831 distributor 076cb5f1 distributor.go:164 │ Registering versions...
//	I0921 10:07:31.759352 distributor 076cb5f1 anneal.go:155      │ Deploying versions...
//	I0921 10:07:31.759696 manager     076cb5f1 manager.go:125     │ Starting versions...
//	I0921 10:07:31.836563 manager     076cb5f1 manager.go:137     │ Success starting...
//	I0921 10:07:31.849647 distributor 076cb5f1 anneal.go:184      │ Successfully deployed...
//	I0921 10:07:31.862637 distributor 076cb5f1 distributor.go:169 │ Successfully registered...
//	I0921 10:07:31.862754 controller  076cb5f1 anneal.go:331      │ Successfully distributed...
func (pp *PrettyPrinter) Format(e *protos.LogEntry) string {
	// We want to pretty print the log entry, preferring prettiness over
	// completeness. We lose some information (e.g., the full filename, the
	// full component name, the full deployment id, the full dinoglet id), but
	// that's okay.
	pp.mu.Lock()
	defer pp.mu.Unlock()
	pp.b.Reset()

	// Write the abbreviated level and time. If the level is "error", we color
	// the level and time. Otherwise, we don't.
	level := " "
	if len(e.Level) > 0 {
		level = strings.ToUpper(e.Level[:1])
	}
	levelColor := colors.Reset
	if strings.ToLower(e.Level) == "error" {
		levelColor = errorColor
	}
	pp.b.WriteString(pp.colorize(levelColor, level))

	cur := time.UnixMicro(e.TimeMicros)
	if cur.Unix() != pp.prevTime.Unix() {
		pp.b.WriteString(pp.colorize(levelColor, cur.Format("0102 15:04:05.000000")))
	} else {
		pp.b.WriteString(pp.colorize(dimColor, cur.Format("0102 15:04:05.")))
		pp.b.WriteString(pp.colorize(levelColor, fmt.Sprintf("%06d", cur.Nanosecond()/1000)))
	}
	pp.b.WriteByte(' ')

	// Write the component.
	c := ShortenComponent(e.Component)
	if len(c) > pp.componentPadding {
		pp.componentPadding = len(c)
	}
	pp.b.WriteString(pp.colorize(colors.ColorHash(c), fmt.Sprintf("%*s", -pp.componentPadding, c)))

	// Write the node.
	if len(e.Node) > 0 {
		pp.b.WriteByte(' ')
		pp.b.WriteString(pp.colorize(colors.ColorHash(e.Node), Shorten(e.Node)))
	}

	// Write the file and line, if present.
	pp.b.WriteByte(' ')
	if e.File != "" && e.Line != -1 {
		file := filepath.Base(e.File)
		line := fmt.Sprint(e.Line)
		if s := fmt.Sprintf("%s:%s", file, line); len(s) > pp.sourcePadding {
			pp.sourcePadding = len(s)
		}

		s := fmt.Sprintf("%s:%s", file, line)
		color := colors.ColorHash(file)
		pp.b.WriteString(fmt.Sprintf("%*s", -pp.sourcePadding-len(color)-len(colors.Reset), pp.colorize(color, s)))
	} else {
		fmt.Fprintf(&pp.b, "%*s", -pp.sourcePadding, "")
	}

	// Write the message.
	pp.b.WriteString(pp.colorize(dimColor, " │ "))
	pp.b.WriteString(e.Msg)

	// Write the attributes, if present.
	for _, attr := range pp.sortedAttributes(e) {
		// Pick attribute color
		var color colors.Code
		if attr[0] == "component" {
			// Stay consistent with color used in component column.
			color = colors.ColorHash(attr[1])
		} else if attr[0] == "err" {
			color = errorColor // Errors have a fixed color
		} else {
			// Assign same color to everything with the same key
			color = colors.ColorHash(attr[0])
		}
		pp.b.WriteString(" ")
		pp.b.WriteString(pp.colorize(dimColor, attr[0]+"="))
		pp.b.WriteString(pp.colorize(color, fmt.Sprintf("%q", attr[1])))
	}

	pp.prevTime = cur
	return pp.b.String()
}

func (pp *PrettyPrinter) sortedAttributes(e *protos.LogEntry) [][2]string {
	attrs := pp.attrs[:0]

	// Get attributes and sort.
	for i := 0; i+1 < len(e.Attrs); i += 2 {
		name, value := e.Attrs[i], e.Attrs[i+1]
		if name == "serviceweaver/system" {
			continue // Aleady implied by component name
		}
		attrs = append(attrs, [2]string{name, value})
	}
	sort.Slice(attrs, func(i, j int) bool {
		return attrs[i][0] < attrs[j][0]
	})

	pp.attrs = attrs // So we reuse allocated space next time
	return attrs
}

// Shorten returns a short prefix of the provided string.
func Shorten(s string) string {
	const n = 8
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// ShortenComponent shortens the given component name to be of the format
// <pkg>.<IfaceType>. (Recall that the full component name is of the format
// <path1>/<path2>/.../<pathN>/<IfaceType>.)
func ShortenComponent(component string) string {
	parts := strings.Split(component, "/")
	switch len(parts) {
	case 0: // should never happen
		return "nil"
	case 1:
		return parts[0]
	default:
		return fmt.Sprintf("%s.%s", parts[len(parts)-2], parts[len(parts)-1])
	}
}
