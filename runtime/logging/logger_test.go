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
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestTestLogger(t *testing.T) {
	// Test plan: Launch a goroutine that continues to write to a TestLogger
	// after the test ends. The logger should stop logging when the test ends.
	t.Run("sub", func(t *testing.T) {
		logger := NewTestSlogger(t, testing.Verbose())
		go func() {
			for {
				logger.Debug("Ping")
				time.Sleep(200 * time.Microsecond)
			}
		}()
		// Give the logger a chance to log something.
		time.Sleep(1 * time.Millisecond)
	})
	// Allow the goroutine to keep running, even though the test has finished.
	time.Sleep(5 * time.Millisecond)
}

func TestWithAttribute(t *testing.T) {
	var got []string
	logSaver := func(e *protos.LogEntry) {
		got = e.Attrs
	}
	logger := newAttrLogger("app", "version", "component", "weavelet", logSaver)
	logger.Info("", "foo", "bar")
	want := []string{"foo", "bar"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("unexpected attributes (-want +got):\n%s", diff)
	}
}

func TestWithAttributes(t *testing.T) {
	var got []string
	logSaver := func(e *protos.LogEntry) {
		got = e.Attrs
	}
	logger := newAttrLogger("app", "version", "component", "weavelet", logSaver)
	logger.Info("", "foo", "bar", "baz", "qux")
	want := []string{"foo", "bar", "baz", "qux"}
	less := func(x, y string) bool { return x < y }
	if diff := cmp.Diff(want, got, cmpopts.SortSlices(less)); diff != "" {
		t.Fatalf("unexpected attributes (-want +got):\n%s", diff)
	}
}

func TestConcurrentAttributes(t *testing.T) {
	// Test plan: start a number of goroutines that emit attributes with sequential
	// values. Confirm that attributes are saved in the same sequential order
	// for each goroutine.
	var mu sync.Mutex
	var allAttributes []string
	logSaver := func(e *protos.LogEntry) {
		if len(e.Attrs) != 2 {
			panic(fmt.Sprintf("too many attributes, want 2, got %d", len(e.Attrs)))
		}
		mu.Lock()
		defer mu.Unlock()
		allAttributes = append(allAttributes, e.Attrs...)
	}
	logger := newAttrLogger("app", "version", "component", "weavelet", logSaver)
	var wait sync.WaitGroup
	const parallelism = 5
	wait.Add(parallelism)
	for i := 0; i < parallelism; i++ {
		i := i
		go func() {
			defer wait.Done()
			l := fmt.Sprintf("routine%d", i)
			for j := 1; j < 100000; j++ {
				logger.Info("", l, fmt.Sprintf("value%d", j))
			}
		}()
	}
	wait.Wait()

	var lastVal [parallelism]int
	for i := 0; i < len(allAttributes)-1; i += 2 {
		attribute := allAttributes[i]
		value := allAttributes[i+1]
		if !strings.HasPrefix(attribute, "routine") {
			t.Fatalf("invalid attribute format, want routine<idx>, got %s", attribute)
		}
		rIdx, err := strconv.Atoi(strings.TrimPrefix(attribute, "routine"))
		if err != nil {
			t.Fatalf("invalid routine index %s: %v", attribute, err)
		}
		if rIdx < 0 || rIdx >= parallelism {
			t.Fatalf("out-of-bound routine index %s", attribute)
		}
		if !strings.HasPrefix(value, "value") {
			t.Fatalf("invalid value format, want value<idx>, got %s", value)
		}
		vIdx, err := strconv.Atoi(strings.TrimPrefix(value, "value"))
		if err != nil {
			t.Fatalf("invalid value index %s: %v", value, err)
		}
		if lastVal[rIdx]+1 != vIdx {
			t.Fatalf("unexpected value index for routine %d, want %d, got %d", rIdx, lastVal[rIdx]+1, vIdx)
		}
		lastVal[rIdx] = vIdx
	}
}

func newAttrLogger(app, version, component, weavelet string, saver func(*protos.LogEntry)) *slog.Logger {
	return slog.New(&LogHandler{
		Opts: Options{
			App:        app,
			Deployment: version,
			Component:  component,
			Weavelet:   weavelet,
		},
		Write: saver,
	})
}
