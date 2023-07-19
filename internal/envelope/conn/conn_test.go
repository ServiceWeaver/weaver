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
	"reflect"
	"strings"
	"testing"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/internal/reflection"
	"github.com/ServiceWeaver/weaver/metrics"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
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

func makeConnections(t *testing.T, handler conn.EnvelopeHandler) (*conn.EnvelopeConn, *conn.WeaveletConn) {
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
	wlet := &protos.EnvelopeInfo{
		App:           "app",
		DeploymentId:  uuid.New().String(),
		Id:            uuid.New().String(),
		SingleProcess: true,
		SingleMachine: true,
	}

	// NOTE: We must start the weavelet conn in a separate goroutine because
	// it initiates a blocking handshake with the envelope.
	ctx, cancel := context.WithCancel(context.Background())
	var w *conn.WeaveletConn
	created := make(chan struct{})
	weaveletDone := make(chan error)
	go func() {
		var err error
		if w, err = conn.NewWeaveletConn(wReader, wWriter); err != nil {
			panic(err)
		}
		created <- struct{}{}
		err = w.Serve(nil)
		weaveletDone <- err

	}()

	e, err := conn.NewEnvelopeConn(ctx, eReader, eWriter, wlet)
	if err != nil {
		t.Fatal(err)
	}
	<-created

	envelopeDone := make(chan error)
	go func() {
		err := e.Serve(handler)
		envelopeDone <- err
	}()

	// Stop the goroutine when test has finished.
	t.Cleanup(func() {
		cancel()
		err := <-envelopeDone
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		err = <-weaveletDone
		if err != nil && !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "file already closed") {
			t.Fatal("weavelet failed", err)
		}
	})

	return e, w
}

type handlerForTest struct{}

var _ conn.EnvelopeHandler = &handlerForTest{}

func (*handlerForTest) HandleTraceSpans(context.Context, *protos.TraceSpans) error {
	return nil
}

func (*handlerForTest) HandleLogEntry(context.Context, *protos.LogEntry) error {
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
	type A interface{}

	type aimpl struct {
		weaver.Implements[A]
	}

	register[A, aimpl]("conn_test/A")
}
