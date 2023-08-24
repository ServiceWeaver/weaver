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
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/protomsg"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/testing/protocmp"
)

const testTimeout = 10 * time.Second

var (
	// matching is a set of test queries that match a non-zero number of log
	// entries written by the loggers returned by the loggers function.
	matching = []Query{
		`app=="test"`,
		`app=="test" && version=="v1"`,
		`app=="test" && component=="a"`,
		`app=="test" && node=="1"`,
		`app=="test" && msg.contains("")`,
		`app=="test" && version=="v1" && component != "b"`,
	}

	// notMatching is a set of test queries that don't match any log entries
	// written by the loggers returned by the loggers function.
	notMatching = []Query{
		`app=="foo"`,
		`app=="test" && version=="v3"`,
		`app=="test" && component=="c"`,
		`app=="test" && node=="5"`,
		`app=="test" && level=="debug"`,
		`app=="test" && msg=="zardoz"`,
	}

	logdir string
)

// opts are the options used to compare two []*Entry.
func opts() []cmp.Option {
	return []cmp.Option{
		cmpopts.SortSlices(func(x, y *protos.LogEntry) bool {
			key := func(e *protos.LogEntry) string {
				return fmt.Sprintf("%s/%s", e.Node, e.Msg)
			}
			return key(x) < key(y)
		}),
		protocmp.Transform(),
		protocmp.IgnoreFields(&protos.LogEntry{}, "time_micros", "file", "line"),
	}
}

// ctx returns a context that expires when t ends or when testTimeout has
// elapsed.
func ctx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	t.Cleanup(cancel)
	return ctx
}

// query is a shorthand for FileSource().Query(ctx, q, follow). The returned
// Reader is automatically closed when t ends.
func query(t *testing.T, ctx context.Context, q Query, follow bool) Reader {
	t.Helper()
	r, err := FileSource(logdir).Query(ctx, q, follow)
	if err != nil {
		t.Fatalf("FileSource().Query(%v, %t): %v", q, follow, err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

// cat is a shorthand for FileSource().Query(ctx, q, false). The returned
// Reader is automatically closed when t ends.
func cat(t *testing.T, ctx context.Context, q Query) Reader {
	t.Helper()
	return query(t, ctx, q, false)
}

// follow is a shorthand for FileSource().Query(ctx, q, true). The returned
// Reader is automatically closed when t ends.
func follow(t *testing.T, ctx context.Context, q Query) Reader {
	t.Helper()
	return query(t, ctx, q, true)
}

type fileTestLogger struct {
	app, dep, component, weavelet string // used to construct a FileLogger
	n                             int    // the number of entries to write
}

// Log logs the messages 0, ..., l.n - 1, sleeping for the provided amount of
// time after every message.
func (l *fileTestLogger) Log(ctx context.Context, sleep time.Duration) error {
	// Make the logger.
	os.MkdirAll(logdir, 0o777)
	fname := filename(l.app, l.dep, l.weavelet, "info")
	file, err := os.Create(filepath.Join(logdir, fname))
	if err != nil {
		return fmt.Errorf("create log file: %q %q %w", logdir, fname, err)
	}
	defer file.Close()

	// Log the entries.
	for i := 0; i < l.n; i++ {
		protomsg.Write(file, &protos.LogEntry{
			App:        l.app,
			Version:    l.dep,
			Component:  l.component,
			Node:       l.weavelet,
			TimeMicros: time.Now().UnixMicro(),
			Level:      "info",
			File:       "",
			Line:       -1,
			Msg:        fmt.Sprint(i),
		})

		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

// Entries returns the entries that are written by Log.
func (l *fileTestLogger) Entries() []*protos.LogEntry {
	entries := make([]*protos.LogEntry, l.n)
	for i := 0; i < l.n; i++ {
		entries[i] = &protos.LogEntry{
			App:        l.app,
			Version:    l.dep,
			Component:  l.component,
			Node:       l.weavelet,
			TimeMicros: time.Now().UnixMicro(),
			Level:      "info",
			File:       "",
			Line:       -1,
			Msg:        strconv.Itoa(i),
		}
	}
	return entries
}

// loggers returns a set of loggers, each of which logs n messages.
func loggers(n int) []*fileTestLogger {
	i := 0
	loggers := []*fileTestLogger{}
	for _, app := range []string{"test"} {
		for _, dep := range []string{"v1", "v2"} {
			for _, c := range []string{"a", "b"} {
				logger := fileTestLogger{
					app:       app,
					dep:       dep,
					component: c,
					weavelet:  strconv.Itoa(i),
					n:         n,
				}
				loggers = append(loggers, &logger)
				i++
			}
		}
	}
	return loggers
}

// entries returns the set of log entries logged by loggers(n) that match q.
func entries(t *testing.T, n int, q Query) []*protos.LogEntry {
	t.Helper()
	env, ast, err := parse(q)
	if err != nil {
		t.Fatal(err)
	}
	prog, err := compile(env, ast)
	if err != nil {
		t.Fatal(err)
	}

	loggers := loggers(n)
	es := []*protos.LogEntry{}
	for _, logger := range loggers {
		for _, e := range logger.Entries() {
			m, err := matches(prog, e)
			if err != nil {
				t.Fatal(err)
			}
			if m {
				es = append(es, e)
			}
		}
	}
	return es
}

func TestFileCatter(t *testing.T) {
	logdir = t.TempDir()
	ctx := ctx(t)

	// Log.
	const n = 100
	for _, logger := range loggers(n) {
		if err := logger.Log(ctx, 0); err != nil {
			t.Fatalf("logger.Log: %v", err)
		}
	}

	for _, q := range matching {
		t.Run(q, func(t *testing.T) {
			// Cat.
			r := cat(t, ctx, q)
			want := entries(t, n, q)
			got := drain(t, ctx, r)
			if diff := cmp.Diff(want, got, opts()...); diff != "" {
				t.Errorf("bad cat (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFileFollowerLogThenFollow(t *testing.T) {
	logdir = t.TempDir()
	ctx := ctx(t)

	// Log.
	const n = 10
	for _, logger := range loggers(n) {
		if err := logger.Log(ctx, 0); err != nil {
			t.Fatalf("logger.Log: %v", err)
		}
	}

	for _, q := range matching {
		t.Run(q, func(t *testing.T) {
			// Follow.
			r := follow(t, ctx, q)
			want := entries(t, n, q)
			got := take(t, ctx, r, len(want))
			if diff := cmp.Diff(want, got, opts()...); diff != "" {
				t.Errorf("bad follow (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFileFollowerFollowThenLog(t *testing.T) {
	for _, q := range matching {
		t.Run(q, func(t *testing.T) {
			logdir = t.TempDir()
			ctx := ctx(t)

			// Log with staggering, after a delay.
			const n = 100
			go func() {
				time.Sleep(10 * time.Millisecond)
				for _, logger := range loggers(n) {
					logger := logger
					go func() { logger.Log(ctx, 1*time.Millisecond) }()
					time.Sleep(25 * time.Millisecond)
				}
			}()

			// Follow, right away.
			r := follow(t, ctx, q)
			want := entries(t, n, q)
			got := take(t, ctx, r, len(want))
			if diff := cmp.Diff(want, got, opts()...); diff != "" {
				t.Errorf("bad follow (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFileFollowerLogAndFollow(t *testing.T) {
	for _, q := range matching {
		t.Run(q, func(t *testing.T) {
			logdir = t.TempDir()
			ctx := ctx(t)

			// Log with staggering, right away.
			const n = 100
			go func() {
				for _, logger := range loggers(n) {
					logger := logger
					go func() { logger.Log(ctx, 1*time.Millisecond) }()
					time.Sleep(25 * time.Millisecond)
				}
			}()

			// Follow, after a delay.
			time.Sleep(50 * time.Millisecond)
			r := follow(t, ctx, q)
			want := entries(t, n, q)
			got := take(t, ctx, r, len(want))
			if diff := cmp.Diff(want, got, opts()...); diff != "" {
				t.Errorf("bad follow (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFileFollowerNoMatches(t *testing.T) {
	for _, q := range notMatching {
		t.Run(q, func(t *testing.T) {
			logdir = t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			defer cancel()

			// Log with staggering, right away.
			const n = 100
			go func() {
				for _, logger := range loggers(n) {
					logger := logger
					go func() { logger.Log(ctx, 0) }()
					time.Sleep(25 * time.Millisecond)
				}
			}()

			// Follow, after a delay. The reader shouldn't match any log
			// entries. It should time out once the context expires and return
			// the context's error.
			time.Sleep(50 * time.Millisecond)
			_, err := follow(t, ctx, q).Read(ctx)
			if err == nil {
				t.Errorf("r.Next(): unexpected success")
			}
			if !errors.Is(err, ctx.Err()) {
				t.Errorf("r.Next(): got %v, want %v", err, ctx.Err())
			}
		})
	}
}

func TestParseLogfile(t *testing.T) {
	for _, want := range []logfile{
		// Simple strings.
		{"a", "b", "c", "d"},

		// Some empty strings.
		{"", "b", "c", "d"},
		{"a", "", "c", "d"},
		{"a", "b", "", "d"},
		{"a", "b", "c", ""},
		{"", "", "c", "d"},
		{"a", "", "", "d"},
		{"a", "b", "", ""},
		{"", "", "", "d"},
		{"a", "", "", ""},
		{"", "", "", ""},

		// Periods.
		{"a", "b.b", "c.c.c", "d.d.d.d"},
		{"a.x", "b", "c", "d"},

		// File suffix.
		{"log", "log", "log", "log"},
	} {
		name := filename(want.app, want.deployment, want.weavelet, want.level)
		t.Run(name, func(t *testing.T) {
			got, err := parseLogfile(name)
			if err != nil {
				t.Fatalf("parseLogFile(name): %v", err)
			}
			if got != want {
				t.Errorf("parseLogFile(name): got %v, want %v", got, want)
			}
		})
	}
}

// drain reads and returns every entry from r.
func drain(t *testing.T, ctx context.Context, r Reader) []*protos.LogEntry {
	t.Helper()
	entries := []*protos.LogEntry{}
	for {
		entry, err := r.Read(ctx)
		if errors.Is(err, io.EOF) {
			return entries
		} else if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}
}

// take reads and returns the first n entries from r.
func take(t *testing.T, ctx context.Context, r Reader, n int) []*protos.LogEntry {
	t.Helper()
	entries := []*protos.LogEntry{}
	for len(entries) < n {
		entry, err := r.Read(ctx)
		if errors.Is(err, io.EOF) {
			return entries
		} else if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}
	return entries
}
