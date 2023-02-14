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
	"errors"
	"io"
	sync "sync"
	"testing"

	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/metrics"
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
	envelope := makeConnections(t)

	// Add metrics on the weavelet side and check that we can observe them in the envelope.
	var metrics map[string]float64
	readMetrics := func() {
		t.Helper()
		list, err := envelope.GetMetricsRPC()
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

func makeConnections(t *testing.T) *conn.EnvelopeConn {
	t.Helper()
	dReader, mWriter := io.Pipe() // envelope -> weavelet
	mReader, dWriter := io.Pipe() // weavelet -> envelope
	h := &handlerForTest{}
	e := conn.NewEnvelopeConn(mReader, mWriter, h)
	d := conn.NewWeaveletConn(dReader, dWriter)

	// Start Run goroutines for both side.
	wait := &sync.WaitGroup{}
	wait.Add(2)
	go func() {
		defer wait.Done()
		err := e.Run()
		if err != nil && !errors.Is(err, io.ErrClosedPipe) && !errors.Is(err, io.EOF) {
			t.Errorf("envelope failed: %v", err)
		}
	}()
	go func() {
		defer wait.Done()
		if _, err := d.GetWeaveletInfo(); err != nil {
			t.Errorf("weavelet failed: %v", err)
		}
		err := d.Run()
		if err != nil && !errors.Is(err, io.ErrClosedPipe) && !errors.Is(err, io.EOF) {
			t.Errorf("weavelet failed: %v", err)
		}
	}()

	// Transfer the Weavelet.
	wlet := &protos.Weavelet{
		Id: uuid.New().String(),
		Dep: &protos.Deployment{
			Id: uuid.New().String(),
			App: &protos.AppConfig{
				Name: "app",
			},
		},
		Group: &protos.ColocationGroup{
			Name: "group",
		},
		GroupReplicaId: uuid.New().String(),
		Process:        "process",
	}
	if err := e.SendWeaveletInfoRPC(wlet); err != nil {
		t.Errorf("envelope failed: %v", err)
	}

	// Stop goroutines when test has finished.
	t.Cleanup(func() {
		dReader.Close()
		mReader.Close()
		wait.Wait()
	})

	return e
}

type handlerForTest struct{}

var _ conn.EnvelopeHandler = &handlerForTest{}

func (h *handlerForTest) RecvTraceSpans([]trace.ReadOnlySpan) error          { return nil }
func (h *handlerForTest) RecvLogEntry(*protos.LogEntry)                      {}
func (h *handlerForTest) StartComponent(*protos.ComponentToStart) error      { return nil }
func (h *handlerForTest) RegisterReplica(*protos.ReplicaToRegister) error    { return nil }
func (h *handlerForTest) StartColocationGroup(*protos.ColocationGroup) error { return nil }
func (h *handlerForTest) ReportLoad(*protos.WeaveletLoadReport) error        { return nil }
func (h *handlerForTest) GetRoutingInfo(*protos.GetRoutingInfo) (*protos.RoutingInfo, error) {
	return nil, nil
}
func (h *handlerForTest) GetComponentsToStart(*protos.GetComponentsToStart) (*protos.ComponentsToStart, error) {
	return nil, nil
}
func (h *handlerForTest) ExportListener(*protos.ListenerToExport) (*protos.ExportListenerReply, error) {
	return nil, nil
}
