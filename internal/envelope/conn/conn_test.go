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

package conn_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/metrics"
	"github.com/ServiceWeaver/weaver/runtime/envelope"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/sdk/trace"
)

// We test the combination of conn, EnvelopeConn, WeaveletConn here.
var (
	c = metrics.NewCounter("TestMetricPropagation.counter", "")
	g = metrics.NewGauge("TestMetricPropagation.gauge", "")
	h = metrics.NewHistogram("TestMetricPropagation.hist", "", []float64{1, 2})
)

func TestMetricPropagation(t *testing.T) {
	envelope, _ := makeConnections(t, &handlerForTest{})

	// Add metrics on the weavelet side and check that we can observe them in the envelope.
	var metrics map[string]float64
	readMetrics := func() {
		t.Helper()
		list, err := envelope.ReadMetrics()
		if err != nil {
			t.Fatalf("metrics fetch: %v", err)
		}
		metrics = map[string]float64{}
		for _, m := range list {
			metrics[m.Name] = m.Value
		}
	}

	checkValue := func(name string, want float64) {
		got, ok := metrics[name]
		if !ok {
			t.Errorf("metric %s not found", name)
		} else if got != want {
			t.Errorf("metric %s: got %v, expecting %v", name, got, want)
		}
	}

	// Update a single metric.
	c.Add(100)
	g.Set(200)
	readMetrics()
	checkValue("TestMetricPropagation.counter", 100)
	checkValue("TestMetricPropagation.gauge", 200)
	checkValue("TestMetricPropagation.hist", 0)

	// Update everything.
	c.Add(200)
	g.Set(500)
	h.Put(1000)
	readMetrics()
	checkValue("TestMetricPropagation.counter", 300)
	checkValue("TestMetricPropagation.gauge", 500)
	checkValue("TestMetricPropagation.hist", 1000)
}

func makeConnections(t *testing.T, handler envelope.EnvelopeHandler) (*envelope.Envelope, *conn.WeaveletConn) {
	t.Helper()

	// Create the pipes. Note that we use os.Pipe instead of io.Pipe. The pipes
	// returned by io.Pipe are synchronous, meaning that a write blocks until a
	// corresponding read (or set of reads) reads the written bytes. This
	// behavior is unlike the normal pipe behavior and complicates things, so
	// we avoid it.
	wReader, eWriter, err := os.Pipe() // envelope -> weavelet
	if err != nil {
		t.Fatal(err)
	}
	eReader, wWriter, err := os.Pipe() // weavelet -> envelope
	if err != nil {
		t.Fatal(err)
	}

	// Construct the conns.
	wlet := &protos.WeaveletSetupInfo{
		App:           "app",
		DeploymentId:  uuid.New().String(),
		Id:            uuid.New().String(),
		SingleProcess: true,
		SingleMachine: true,
	}

	// Start Run goroutines for both conns.
	started := &sync.WaitGroup{}
	started.Add(2)
	done := &sync.WaitGroup{}
	done.Add(2)
	var e *envelope.Envelope
	go func() {
		defer done.Done()

		ctx := context.WithValue(context.Background(), conn.ContextKey, conn.IO{
			Reader: eReader,
			Writer: eWriter,
		})
		var err error
		if e, err = envelope.NewEnvelope(ctx, wlet, &protos.AppConfig{}, handler); err != nil {
			panic(err)
		}
		started.Done()
		if err := e.Serve(ctx); err != nil && !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "file already closed") {
			t.Errorf("envelope failed: %#v", err)
		}
	}()
	var w *conn.WeaveletConn
	go func() {
		defer done.Done()
		var err error
		if w, err = conn.NewWeaveletConn(wReader, wWriter, nil /*handler*/); err != nil {
			panic(err)
		}
		started.Done()
		if err := w.Serve(); err != nil && !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "file already closed") {
			t.Errorf("weavelet failed: %#v", err)
		}
	}()

	// Wait for both conns to start.
	started.Wait()

	// Stop goroutines when test has finished.
	t.Cleanup(func() {
		wReader.Close()
		eReader.Close()
		// NOTE(mwhittaker): wWriter and eWriter are closed by the conns.
		done.Wait()
	})

	return e, w
}

type handlerForTest struct{}

var _ envelope.EnvelopeHandler = &handlerForTest{}

func (h *handlerForTest) RecvTraceSpans([]trace.ReadOnlySpan) error     { return nil }
func (h *handlerForTest) RecvLogEntry(*protos.LogEntry)                 {}
func (h *handlerForTest) StartComponent(*protos.ComponentToStart) error { return nil }
func (h *handlerForTest) GetAddress(*protos.GetAddressRequest) (*protos.GetAddressReply, error) {
	return nil, nil
}
func (h *handlerForTest) ExportListener(*protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
	return nil, nil
}
