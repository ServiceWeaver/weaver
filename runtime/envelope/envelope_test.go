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

package envelope

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/retry"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// The result of running os.Executable(). Populated by TestMain.
var executable = ""

func TestMain(m *testing.M) {
	// The tests in this file run the test binary as subprocesses with a
	// subcommand (e.g., "loop", "succeed"). When run as a subprocess, this
	// test binary prints its pid, performs the specified subcommand, and
	// exits. It does not run any of the tests.
	flag.Parse()
	cmd := flag.Arg(0)

	if cmd != "" {
		// Note that subprocess is not starting a weavelet, so we have to manually
		// create a weavelet conn. Otherwise, when the envelope runs, it will fail
		// because it's unable to send the weavelet information to the subprocess.
		conn, err := createWeaveletConn()
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to create weavelet conn for subprocess: %v\n", err)
			os.Exit(1)
		}

		cmds := map[string]func() error{
			"loop": func() error {
				for {
					time.Sleep(10 * time.Millisecond)
				}
			},
			"succeed": func() error { return nil },
			"fail":    func() error { os.Exit(1); return nil },
			"flip":    failOften,
			"file":    checkFile,
			"bigprint": func() error {
				n, err := strconv.Atoi(flag.Arg(1))
				if err != nil {
					return err
				}
				// -1 because the pid takes up one line
				return bigprint(n - 1)
			},
			"writetraces": func() error { return writeTraces(conn) },
			"serve_conn":  func() error { return conn.Run() },
		}
		fn, ok := cmds[cmd]
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
			os.Exit(1)
		}
		fmt.Println(os.Getpid())
		err = fn()
		if err != nil {
			fmt.Fprintf(os.Stderr, "subprocess: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	var err error
	executable, err = os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// failOften fails with high probability.
func failOften() error {
	rand.Seed(time.Now().UnixNano())
	if rand.Intn(10) != 0 {
		os.Exit(1)
	}
	return nil
}

// checkFile succeeds if the given file exists, or makes the file and fails.
func checkFile() error {
	if _, err := os.Stat(flag.Arg(1)); err == nil {
		return nil
	}
	if _, err := os.Create(flag.Arg(1)); err != nil {
		return err
	}
	os.Exit(1)
	return nil
}

// wlet returns a WeaveletInfo and AppConfig for testing.
func wlet(binary string, args ...string) (*protos.WeaveletInfo, *protos.AppConfig) {
	weavelet := &protos.WeaveletInfo{
		App:           "app",
		DeploymentId:  uuid.New().String(),
		Group:         &protos.ColocationGroup{Name: "main"},
		GroupId:       uuid.New().String(),
		Id:            uuid.New().String(),
		SingleProcess: true,
		SingleMachine: true,
	}
	config := &protos.AppConfig{Binary: binary, Args: args}
	return weavelet, config
}

// pidSaver is a log saver that parses and stores pids from the log entries'
// messages.
type pidSaver struct {
	mu   sync.Mutex
	pids map[int]bool
}

func (p *pidSaver) save(entry *protos.LogEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pid, err := strconv.Atoi(entry.Msg)
	if err != nil {
		panic(err)
	}
	p.pids[pid] = true
}

// testSaver returns a log saver that pretty prints logs using t.Log.
func testSaver(t *testing.T) func(entry *protos.LogEntry) {
	pp := logging.NewPrettyPrinter(colors.Enabled())
	return func(entry *protos.LogEntry) {
		t.Log(pp.Format(entry))
	}
}

type handlerForTest struct {
	logSaver func(*protos.LogEntry)

	mu     sync.Mutex
	traces []string
}

var _ EnvelopeHandler = &handlerForTest{}

func (h *handlerForTest) RecvTraceSpans(spans []sdktrace.ReadOnlySpan) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, span := range spans {
		h.traces = append(h.traces, span.Name())
	}
	return nil
}

func (h *handlerForTest) getTraceSpanNames() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.traces
}

func (h *handlerForTest) RecvLogEntry(entry *protos.LogEntry)             { h.logSaver(entry) }
func (h *handlerForTest) StartComponent(*protos.ComponentToStart) error   { return nil }
func (h *handlerForTest) RegisterReplica(*protos.ReplicaToRegister) error { return nil }
func (h *handlerForTest) ReportLoad(*protos.WeaveletLoadReport) error     { return nil }
func (h *handlerForTest) GetRoutingInfo(*protos.GetRoutingInfo) (*protos.RoutingInfo, error) {
	return nil, nil
}
func (h *handlerForTest) GetComponentsToStart(*protos.GetComponentsToStart) (*protos.ComponentsToStart, error) {
	return nil, nil
}
func (h *handlerForTest) GetAddress(*protos.GetAddressRequest) (*protos.GetAddressReply, error) {
	return nil, nil
}
func (h *handlerForTest) ExportListener(*protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
	return nil, nil
}

func TestRun(t *testing.T) {
	for _, test := range []struct {
		subcommand string
		args       []string
		restart    RestartPolicy
		succeed    bool
		minPids    int
		maxPids    int
	}{
		{"succeed", []string{}, Never, true, 1, 1},
		{"succeed", []string{}, OnFailure, true, 1, 1},
		{"fail", []string{}, Never, false, 1, 1},
		{"flip", []string{}, OnFailure, true, 1, -1},
		{"file", []string{filepath.Join(t.TempDir(), "file.txt")}, OnFailure, true, 2, 2},
	} {
		name := fmt.Sprintf("%v/%v", test.subcommand, test.restart)
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			pids := pidSaver{pids: map[int]bool{}}
			opts := Options{
				Restart: test.restart,
				// Avoid long backoffs.
				Retry: retry.Options{
					BackoffMultiplier:  1,
					BackoffMinDuration: time.Millisecond,
				},
			}
			args := append([]string{test.subcommand}, test.args...)
			wlet, config := wlet(executable, args...)
			e, err := NewEnvelope(wlet, config, &handlerForTest{logSaver: pids.save}, opts)
			if err != nil {
				t.Fatal(err)
			}

			err = e.Run(ctx)
			if err != nil && test.succeed {
				t.Fatalf("m.Run(): %v", err)
			}
			if err == nil && !test.succeed {
				t.Fatal("m.Run(): unexpected success")
			}

			n := len(pids.pids)
			if n < test.minPids {
				t.Fatalf("got %d pids, want at least %d", n, test.minPids)
			}
			if test.maxPids != -1 && n > test.maxPids {
				t.Fatalf("got %d pids, want at most %d", n, test.maxPids)
			}
		})
	}
}

// bigprint prints out n large lines of text and then fails.
func bigprint(n int) error {
	s := strings.Repeat("x", 1000)
	for i := 0; i < n; i++ {
		fmt.Println(s)
	}
	os.Exit(1)
	return nil
}

func TestBigPrints(t *testing.T) {
	ctx := context.Background()
	var entries []*protos.LogEntry
	var m sync.Mutex
	h := &handlerForTest{logSaver: func(entry *protos.LogEntry) {
		m.Lock()
		defer m.Unlock()
		entries = append(entries, entry)
	}}

	n := 10000
	wlet, config := wlet(executable, "bigprint", strconv.Itoa(n))
	e, err := NewEnvelope(wlet, config, h, Options{Restart: Never})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Run(ctx); err == nil {
		t.Fatalf("dm.Run(): unexpected success")
	}

	var got int
	m.Lock()
	got = len(entries)
	m.Unlock()
	if got != n {
		t.Fatalf("got %d log entries, want %d", got, n)
	}
}

// TestCancel test that a envelope run() loop is canceled when the passed-in
// context is canceled.
func TestCancel(t *testing.T) {
	for _, restart := range []RestartPolicy{
		Never,
		Always,
		OnFailure,
	} {
		name := fmt.Sprintf("%v", restart)
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			wlet, config := wlet(executable, "loop")
			e, err := NewEnvelope(wlet, config,
				&handlerForTest{logSaver: testSaver(t)}, Options{})
			if err != nil {
				t.Fatal(err)
			}

			// End the capture after a delay.
			go func() {
				time.Sleep(100 * time.Millisecond)
				cancel()
			}()

			// Start the subprocess. It should be ended after a delay.
			if err := e.Run(ctx); err == nil {
				t.Fatal("m.Run(): unexpected success")
			}

			// Double check that the subprocess was killed.
			ps := exec.Command("pgrep", "-f", "capture.test loop")
			output, err := ps.Output()
			// NOTE: If nothing matches, pgrep returns an "exit status 1" error
			// with an empty output.
			if err != nil && err.Error() != "exit status 1" {
				t.Fatalf("cannot pgrep: %v", err)
			}
			if len(output) > 0 {
				t.Fatalf("capture.test subprocess still running")
			}
		})
	}
}

func createWeaveletConn() (*conn.WeaveletConn, error) {
	bootstrap, err := runtime.GetBootstrap(context.Background())
	if err != nil {
		return nil, fmt.Errorf("unable to get pipe info from env: %w", err)
	}
	toWeavelet, toEnvelope, err := bootstrap.MakePipes()
	if err != nil {
		return nil, fmt.Errorf("unable make weavelet<->envelope pipes: %w", err)
	}
	return conn.NewWeaveletConn(toWeavelet, toEnvelope)
}

func writeTraces(conn *conn.WeaveletConn) error {
	w := traceio.NewWriter(conn.SendTraceSpans)
	defer w.Shutdown(context.Background())

	span := func(name string) sdktrace.ReadOnlySpan {
		rnd := uuid.New()
		return &traceio.ReadSpan{Span: &protos.Span{
			Name:         name,
			TraceId:      rnd[:16],
			SpanId:       rnd[:8],
			ParentSpanId: rnd[:8],
		}}
	}
	if err := w.ExportSpans(context.Background(), []sdktrace.ReadOnlySpan{
		span("span1"),
		span("span2"),
	}); err != nil {
		return err
	}
	if err := w.ExportSpans(context.Background(), []sdktrace.ReadOnlySpan{
		span("span3"),
		span("span4"),
	}); err != nil {
		return err
	}
	return nil
}

func TestTraces(t *testing.T) {
	h := &handlerForTest{logSaver: testSaver(t)}
	ctx := context.Background()
	wlet, config := wlet(executable, "writetraces")
	e, err := NewEnvelope(wlet, config, h, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Run(ctx); err != nil {
		t.Fatalf("unexpected error from child: %v", err)
	}
	expect := []string{"span1", "span2", "span3", "span4"}
	actual := h.getTraceSpanNames()
	if diff := cmp.Diff(expect, actual); diff != "" {
		t.Errorf("traces diff: (-want,+got):\n%s", diff)
	}
}

func startEnvelopeWithServing(ctx context.Context, t *testing.T) *Envelope {
	h := &handlerForTest{logSaver: testSaver(t)}
	wlet, config := wlet(executable, "serve_conn")
	e, err := NewEnvelope(wlet, config, h, Options{})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		if err := e.Run(ctx); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()

	// Wait until the envelope and the weavelet are available.
	for {
		if e.getConn() != nil {
			break
		}
	}
	return e
}

func TestSingleProfile(t *testing.T) {
	// Test Plan: Start an envelope and send it a heap and a cpu profile
	// requests, one after another. Verify that both succeed and return
	// non-empty profile data.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := startEnvelopeWithServing(ctx, t)

	for _, typ := range []protos.ProfileType{protos.ProfileType_Heap, protos.ProfileType_CPU} {
		typ := typ
		t.Run(typ.String(), func(t *testing.T) {
			// Send a profiling request and wait for a reply.
			req := &protos.RunProfiling{
				ProfileType:   typ,
				CpuDurationNs: int64(100 * time.Millisecond / time.Nanosecond),
			}
			reply, err := e.RunProfiling(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			if len(reply.Errors) > 0 {
				t.Fatalf("profiling error: %v", reply.Errors)
			}
			if len(reply.Data) == 0 {
				t.Fatal("empty profile data")
			}
		})
	}
}

func TestConcurrentProfiles(t *testing.T) {
	// Test Plan: Start an envelope and send it two concurrent cpu profile
	// requests. Verify that one of the requests returns an
	// "already in progress" error.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := startEnvelopeWithServing(ctx, t)

	prof := func() error {
		req := &protos.RunProfiling{
			ProfileType:   protos.ProfileType_CPU,
			CpuDurationNs: int64(9999 * time.Hour / time.Nanosecond), // ensure overlap
		}
		_, err := e.RunProfiling(ctx, req)
		return err
	}

	// Issue two concurrent profiling requests: one should fail.
	var wait sync.WaitGroup
	var profMu sync.Mutex
	var profErr error
	wait.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wait.Done()
			defer cancel()
			if err := prof(); err != nil {
				profMu.Lock()
				defer profMu.Unlock()
				if profErr == nil {
					profErr = err
				}
			}
		}()
	}
	wait.Wait()

	profMu.Lock()
	defer profMu.Unlock()
	const expect = "already in progress"
	if profErr == nil || !strings.Contains(profErr.Error(), expect) {
		t.Fatalf("unexpected profiler error, want %s got %v", expect, profErr)
	}
}
