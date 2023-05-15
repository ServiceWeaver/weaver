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

// Package logging contains logging related utilities.
package logging

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/uuid"
	"golang.org/x/exp/slog"
)

// Options configures the log entries produced by a logger.
type Options struct {
	App        string // Service Weaver application (e.g., "todo")
	Deployment string // Service Weaver deployment (e.g., "36105c89-85b1...")
	Component  string // Service Weaver component (e.g., "Todo")
	Weavelet   string // Service Weaver weavelet id (e.g., "36105c89-85b1...")

	// Pre-assigned attributes. These will be attached to each log entry
	// generated by the logger. This slice will never be appended to in place.
	Attrs []string
}

// LogHandler implements a custom slog.Handler.
type LogHandler struct {
	Opts  Options                      // configures the log entries
	Write func(entry *protos.LogEntry) // called on every log entry
}

var _ slog.Handler = &LogHandler{}

// Handle implements the slog.Handler interface.
func (h *LogHandler) Handle(ctx context.Context, rec slog.Record) error {
	h.Write(h.makeEntry(rec))
	return nil
}

// Enabled implements the slog.Handler interface.
func (h *LogHandler) Enabled(context.Context, slog.Level) bool {
	// Support all logging levels.
	return true
}

// WithAttrs implements the slog.Handler interface.
func (h *LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Note that attributes set through explicit calls to log.WithAttrs should
	// apply to all the log entries, similar to h.Opts.
	//
	// Note that WithAttrs results in a new logger, hence we should create a new
	// handler that contains the new attributes.
	rh := &LogHandler{
		Opts:  h.Opts,
		Write: h.Write,
	}
	rh.Opts.Attrs = appendAttrs(rh.Opts.Attrs, attrs)
	return rh
}

// WithGroup implements the slog.Handler interface.
//
// TODO(rgrandl): Implement it, so the users have the same experience as slog
// if they decide to use WithGroup.
func (h *LogHandler) WithGroup(string) slog.Handler {
	return h
}

// makeEntry returns an entry that is fully populated with information captured
// by a log record.
func (h *LogHandler) makeEntry(rec slog.Record) *protos.LogEntry {
	// TODO(sanjay): Is it necessary to copy opts.Attrs even if no new attrs
	// are being added?
	var attrs []slog.Attr
	rec.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})

	entry := protos.LogEntry{
		App:        h.Opts.App,
		Version:    h.Opts.Deployment,
		Component:  h.Opts.Component,
		Node:       h.Opts.Weavelet,
		TimeMicros: rec.Time.UnixMicro(),
		Level:      rec.Level.String(),
		File:       "",
		Line:       -1,
		Msg:        rec.Message,
		Attrs:      appendAttrs(h.Opts.Attrs, attrs),
	}

	// Get the file and line information.
	fs := runtime.CallersFrames([]uintptr{rec.PC})
	if fs != nil {
		frame, _ := fs.Next()
		entry.File = frame.File
		entry.Line = int32(frame.Line)
	}
	return &entry
}

// StderrLogger returns a logger that pretty prints log entries to stderr.
func StderrLogger(opts Options) *slog.Logger {
	pp := NewPrettyPrinter(colors.Enabled())
	writeText := func(entry *protos.LogEntry) {
		fmt.Fprintln(os.Stderr, pp.Format(entry))
	}
	return slog.New(&LogHandler{Opts: opts, Write: writeText})
}

// TB is the subset of the [testing.TB] interface needed by a TestLogger.
type TB interface {
	Log(args ...any)
	Cleanup(func())
}

// TestLogger implements a logger for tests.
type TestLogger struct {
	t        TB         // logs until t finishes
	verbose  bool       // show logs?
	mu       sync.Mutex // guards finished
	finished bool       // has t finished?
}

// Log logs the provided log entry using t.t.Log while the test is running and
// logs to stderr afterwards.
func (t *TestLogger) Log(entry *protos.LogEntry) {
	if entry.TimeMicros == 0 {
		entry.TimeMicros = time.Now().UnixMicro()
	}

	// We create a new pretty printer for every message to disable dimming and
	// indenting. When multiple log outputs from different weavelets are
	// intermixed in a test, dimming and indenting can get confusing.
	msg := NewPrettyPrinter(colors.Enabled()).Format(entry)

	t.mu.Lock()
	defer t.mu.Unlock()
	switch {
	case !t.verbose:
		// If -test.v isn't provided, don't log anything. Note that writing to
		// t.Log isn't sufficient because B.Log ignores -test.v [1]. We
		// override this behavior because, otherwise, weavertest benchmark
		// results are polluted with logs and are very hard to read.
		//
		// Also note that we still format msg and grab the lock even if
		// t.verbose is false to prevent the performance of a benchmark varying
		// drastically based on the presence of the -test.v flag.
		//
		// [1]: https://pkg.go.dev/testing#B.Log
	case t.finished:
		// If the test is finished, Log may panic if called. Instead, we write
		// to stderr so we don't lose useful debugging information.
		fmt.Fprintln(os.Stderr, msg)
	default:
		t.t.Log(msg)
	}
}

// NewTestLogger returns a new TestLogger.
func NewTestLogger(t TB, verbose bool) *TestLogger {
	logger := &TestLogger{t: t, verbose: verbose}
	t.Cleanup(func() {
		logger.mu.Lock()
		defer logger.mu.Unlock()
		logger.finished = true
	})
	return logger
}

// NewTestSlogger returns a new logger for tests.
func NewTestSlogger(t TB, verbose bool) *slog.Logger {
	logger := NewTestLogger(t, verbose)
	return slog.New(&LogHandler{
		Opts:  Options{Component: "TestLogger", Weavelet: uuid.New().String()},
		Write: logger.Log,
	})
}

// appendAttrs appends <name,value> pairs found in attrs to prefix
// and returns the resulting slice. It never appends in place.
func appendAttrs(prefix []string, attrs []slog.Attr) []string {
	if len(attrs) == 0 {
		return prefix
	}

	// NOTE: Copy prefix to avoid the scenario where two different
	// loggers overwrite the existing slice entry. This is possible,
	// for example, if two goroutines call With() on the same logger
	// concurrently.
	var dst []string
	dst = append(dst, prefix...)

	// Extract key,value pairs from attrs.
	for _, attr := range attrs {
		dst = append(dst, []string{attr.Key, attr.Value.String()}...)
	}
	return dst
}
