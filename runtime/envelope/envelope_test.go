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
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/internal/reflection"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/retry"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// The result of running os.Executable(). Populated by TestMain.
var executable = ""

func TestMain(m *testing.M) {
	// The tests in this file run the test binary as subprocesses with a
	// subcommand (e.g., "loop", "fail"). When run as a subprocess, this
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
				// Default behavior of blocking forever.
				return nil
			},
			"fail":       func() error { os.Exit(1); return nil },
			"check_file": checkFile,
			"bigprint": func() error {
				n, err := strconv.Atoi(flag.Arg(1))
				if err != nil {
					return err
				}
				// -1 because the pid takes up one line
				if err := bigprint(n - 1); err != nil {
					return err
				}
				os.Exit(1)
				return nil
			},
			"serve_conn": func() error { return conn.Serve(context.Background()) },
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
		conn.Serve(context.Background())
	}

	var err error
	executable, err = os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// checkFile succeeds iff the given file exists.
func checkFile() error {
	_, err := os.Stat(flag.Arg(1))
	return err
}

// wlet returns a EnvelopeInfo and AppConfig for testing.
func wlet(binary string, args ...string) (*protos.EnvelopeInfo, *protos.AppConfig) {
	weavelet := &protos.EnvelopeInfo{
		App:             "app",
		DeploymentId:    uuid.New().String(),
		Id:              uuid.New().String(),
		InternalAddress: "localhost:0",
	}
	config := &protos.AppConfig{Binary: binary, Args: args}
	return weavelet, config
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

func (h *handlerForTest) HandleTraceSpans(_ context.Context, spans *protos.TraceSpans) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, span := range spans.Span {
		h.traces = append(h.traces, span.Name)
	}
	return nil
}

func (*handlerForTest) LogBatch(context.Context, *protos.LogEntryBatch) error {
	return nil
}

func (h *handlerForTest) HandleLogEntry(_ context.Context, entry *protos.LogEntry) error {
	h.logSaver(entry)
	return nil
}

func (*handlerForTest) ActivateComponent(context.Context, *protos.ActivateComponentRequest) (*protos.ActivateComponentReply, error) {
	return nil, nil
}

func (*handlerForTest) GetListenerAddress(context.Context, *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error) {
	return nil, nil
}

func (*handlerForTest) ExportListener(context.Context, *protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
	return nil, nil
}

func (*handlerForTest) GetSelfCertificate(context.Context, *protos.GetSelfCertificateRequest) (*protos.GetSelfCertificateReply, error) {
	panic("unused")
}

func (*handlerForTest) VerifyClientCertificate(context.Context, *protos.VerifyClientCertificateRequest) (*protos.VerifyClientCertificateReply, error) {
	panic("unused")
}

func (*handlerForTest) VerifyServerCertificate(context.Context, *protos.VerifyServerCertificateRequest) (*protos.VerifyServerCertificateReply, error) {
	panic("unused")
}

// TestWeaveletExit tests that an envelope captures the stdout and stderr of a
// subprocess that fails to perform the envelope-weavelet handshake.
func TestWeaveletExit(t *testing.T) {
	ctx := context.Background()
	wlet, config := wlet("/usr/bin/env", "bash", "-c", `echo "hello from stdout"; echo "hello from stderr" >&2 && exit 1`)
	_, err := NewEnvelope(ctx, wlet, config, Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "hello from stdout") {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "hello from stderr") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStartStop(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "file.txt")
	if _, err := os.Create(filename); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		subcommand string
		args       []string
		fail       bool
	}{
		{"loop", []string{}, false},
		{"fail", []string{}, true},
		{"check_file", []string{filename}, false},
	} {
		name := test.subcommand
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			var started atomic.Bool
			args := append([]string{test.subcommand}, test.args...)
			wlet, config := wlet(executable, args...)
			e, err := NewEnvelope(ctx, wlet, config, Options{})
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error)
			go func() {
				h := &handlerForTest{
					logSaver: func(entry *protos.LogEntry) {
						started.Store(true)
					},
				}
				err := e.Serve(h)
				done <- err
			}()

			// Wait for the weavelet to start.
			for r := retry.Begin(); !started.Load() && r.Continue(ctx); {
			}
			if ctx.Err() != nil {
				t.Fatalf("timeout waiting for weavelet to start")
			}

			if !test.fail {
				// Give the weavelet a chance to fail, and verify that it didn't.
				time.Sleep(200 * time.Millisecond)
				cancel()
				if err := <-done; !errors.Is(err, context.Canceled) {
					t.Fatalf("weavelet failed: %v", err)
				}
			} else {
				// Wait for the weavelet to fail.
				if err := <-done; errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("weavelet didn't fail: %v", err)
				}
				cancel()
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
	return nil
}

func TestBigPrints(t *testing.T) {
	// Test Plan: Start a weavelet that prints a bunch of messages and then
	// exists, simulating a panic(). Make sure that all messages are received.
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
	e, err := NewEnvelope(ctx, wlet, config, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Serve(h); errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("deadline exceeded error")
	}

	var got int
	m.Lock()
	got = len(entries)
	m.Unlock()
	if got != n {
		t.Fatalf("got %d log entries, want %d", got, n)
	}
}

// TestCancel test that a weavelet process is stopped when the passed-in
// context is canceled.
func TestCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	wlet, config := wlet(executable, "loop")
	e, err := NewEnvelope(ctx, wlet, config, Options{})
	if err != nil {
		t.Fatal(err)
	}

	// Stop the envelope after a delay.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	// Start the subprocess. It should be ended after a delay.
	h := &handlerForTest{logSaver: testSaver(t)}
	if err := e.Serve(h); !errors.Is(err, context.Canceled) {
		t.Fatal("weavelet failed unexpectedly")
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

type A interface{}

type aimpl struct {
	weaver.Implements[A]
}

func register[Intf, Impl any](name string) {
	var zero Impl
	codegen.Register(codegen.Registration{
		Name:         name,
		Iface:        reflection.Type[Intf](),
		Impl:         reflect.TypeOf(zero),
		LocalStubFn:  func(any, string, trace.Tracer) any { return nil },
		ClientStubFn: func(codegen.Stub, string) any { return nil },
		ServerStubFn: func(any, func(uint64, float64)) codegen.Server { return nil },
	})
}

// Register a dummy component for test.
func init() {
	register[A, aimpl]("envelope_test/A")
}
