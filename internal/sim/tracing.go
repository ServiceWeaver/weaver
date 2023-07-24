package sim

import (
	"context"
	"encoding/binary"
	"sync"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type exporter struct {
	mu    sync.Mutex
	spans []sdktrace.ReadOnlySpan
}

func (e *exporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = append(e.spans, spans...)
	return nil
}

func (e *exporter) Shutdown(ctx context.Context) error {
	return nil
}

type idGenerator struct {
	mu      sync.Mutex
	traceID uint64
	spanID  uint64
}

func (i *idGenerator) NewIDs(ctx context.Context) (trace.TraceID, trace.SpanID) {
	i.mu.Lock()
	defer i.mu.Unlock()
	var traceID [16]byte
	var spanID [8]byte
	binary.BigEndian.PutUint64(traceID[8:], i.traceID)
	binary.BigEndian.PutUint64(spanID[:], i.spanID)
	i.traceID++
	i.spanID++
	return traceID, spanID
}

func (i *idGenerator) NewSpanID(ctx context.Context, traceID trace.TraceID) trace.SpanID {
	i.mu.Lock()
	defer i.mu.Unlock()
	var spanID [8]byte
	binary.BigEndian.PutUint64(spanID[:], i.spanID)
	i.spanID++
	return spanID
}
