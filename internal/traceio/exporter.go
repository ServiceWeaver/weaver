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

package traceio

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/protos"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	// Interval at which traces are exported.
	ExportInterval = 5 * time.Second
)

// Writer writes a sequence of trace spans to a specified export function.
type Writer struct {
	mu     sync.Mutex
	export func(spans *protos.TraceSpans) error
}

// NewWriter creates a Writer that writes a sequence of trace spans to a
// specified export function.
func NewWriter(export func(spans *protos.TraceSpans) error) *Writer { return &Writer{export: export} }

var _ sdk.SpanExporter = &Writer{}

// ExportSpans implements the sdk.SpanExporter interface.
func (w *Writer) ExportSpans(_ context.Context, spans []sdk.ReadOnlySpan) error {
	msg := &protos.TraceSpans{}
	msg.Span = make([]*protos.Span, len(spans))
	for i, span := range spans {
		msg.Span[i] = toProtoSpan(span)
	}
	return w.ExportSpansProto(msg)
}

// ExportSpansProto is like ExportSpans, but it exports spans in the
// *protos.TraceSpans format.
func (w *Writer) ExportSpansProto(spans *protos.TraceSpans) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.export(spans)
}

// Shutdown implements the sdk.SpanExporter interface.
func (w *Writer) Shutdown(_ context.Context) error {
	return nil
}

func toProtoSpan(span sdk.ReadOnlySpan) *protos.Span {
	tid := span.SpanContext().TraceID()
	sid := span.SpanContext().SpanID()
	psid := span.Parent().SpanID()
	return &protos.Span{
		Name:                  span.Name(),
		TraceId:               tid[:],
		SpanId:                sid[:],
		ParentSpanId:          psid[:],
		Kind:                  toProtoKind(span.SpanKind()),
		StartMicros:           span.StartTime().UnixMicro(),
		EndMicros:             span.EndTime().UnixMicro(),
		Attributes:            toProtoAttrs(span.Attributes()),
		Links:                 toProtoLinks(span.Links()),
		Events:                toProtoEvents(span.Events()),
		Status:                toProtoStatus(span.Status()),
		Scope:                 toProtoScope(span.InstrumentationScope()),
		Library:               toProtoLibrary(span.InstrumentationLibrary()),
		Resource:              toProtoResource(span.Resource()),
		DroppedAttributeCount: int64(span.DroppedAttributes()),
		DroppedLinkCount:      int64(span.DroppedLinks()),
		DroppedEventCount:     int64(span.DroppedEvents()),
		ChildSpanCount:        int64(span.ChildSpanCount()),
	}
}

func toProtoKind(kind trace.SpanKind) protos.Span_Kind {
	switch kind {
	case trace.SpanKindUnspecified:
		return protos.Span_UNSPECIFIED
	case trace.SpanKindInternal:
		return protos.Span_INTERNAL
	case trace.SpanKindServer:
		return protos.Span_SERVER
	case trace.SpanKindClient:
		return protos.Span_CLIENT
	case trace.SpanKindProducer:
		return protos.Span_PRODUCER
	case trace.SpanKindConsumer:
		return protos.Span_CONSUMER
	default:
		return protos.Span_INTERNAL
	}
}

func toProtoAttrs(kvs []attribute.KeyValue) []*protos.Span_Attribute {
	if len(kvs) == 0 {
		return nil
	}
	attrs := make([]*protos.Span_Attribute, len(kvs))
	for i, kv := range kvs {
		attr := &protos.Span_Attribute{
			Key:   string(kv.Key),
			Value: &protos.Span_Attribute_Value{},
		}
		switch kv.Value.Type() {
		case attribute.BOOL:
			attr.Value.Type = protos.Span_Attribute_Value_BOOL
			val := &protos.Span_Attribute_Value_Num{Num: 0}
			if kv.Value.AsBool() {
				val.Num = 1
			}
			attr.Value.Value = val
		case attribute.INT64:
			attr.Value.Type = protos.Span_Attribute_Value_INT64
			attr.Value.Value = &protos.Span_Attribute_Value_Num{Num: uint64(kv.Value.AsInt64())}
		case attribute.FLOAT64:
			attr.Value.Type = protos.Span_Attribute_Value_FLOAT64
			attr.Value.Value = &protos.Span_Attribute_Value_Num{Num: math.Float64bits(kv.Value.AsFloat64())}
		case attribute.STRING:
			attr.Value.Type = protos.Span_Attribute_Value_STRING
			attr.Value.Value = &protos.Span_Attribute_Value_Str{Str: kv.Value.AsString()}
		case attribute.BOOLSLICE:
			// TODO(spetrovic): Store as a bitset.
			attr.Value.Type = protos.Span_Attribute_Value_BOOLLIST
			vals := kv.Value.AsBoolSlice()
			b := make([]byte, len(vals))
			for i, v := range vals {
				if v {
					b[i] = 1
				}
			}
			attr.Value.Value = &protos.Span_Attribute_Value_Str{Str: string(b)}
		case attribute.INT64SLICE:
			attr.Value.Type = protos.Span_Attribute_Value_INT64LIST
			vals := kv.Value.AsInt64Slice()
			nums := make([]uint64, len(vals))
			for i, v := range vals {
				nums[i] = uint64(v)
			}
			attr.Value.Value = &protos.Span_Attribute_Value_Nums{Nums: &protos.Span_Attribute_Value_NumberList{Nums: nums}}
		case attribute.FLOAT64SLICE:
			attr.Value.Type = protos.Span_Attribute_Value_FLOAT64LIST
			vals := kv.Value.AsFloat64Slice()
			nums := make([]uint64, len(vals))
			for i, v := range vals {
				nums[i] = math.Float64bits(v)
			}
			attr.Value.Value = &protos.Span_Attribute_Value_Nums{Nums: &protos.Span_Attribute_Value_NumberList{Nums: nums}}
		case attribute.STRINGSLICE:
			attr.Value.Type = protos.Span_Attribute_Value_STRINGLIST
			vals := kv.Value.AsStringSlice()
			strs := make([]string, len(vals))
			copy(strs, vals)
			attr.Value.Value = &protos.Span_Attribute_Value_Strs{Strs: &protos.Span_Attribute_Value_StringList{Strs: strs}}
		default:
			attr.Value.Type = protos.Span_Attribute_Value_INVALID
		}
		attrs[i] = attr
	}
	return attrs
}

func toProtoLinks(links []sdk.Link) []*protos.Span_Link {
	if len(links) == 0 {
		return nil
	}
	pl := make([]*protos.Span_Link, len(links))
	for i, l := range links {
		tid := l.SpanContext.TraceID()
		sid := l.SpanContext.SpanID()
		pl[i] = &protos.Span_Link{
			TraceId:               tid[:],
			SpanId:                sid[:],
			Attributes:            toProtoAttrs(l.Attributes),
			DroppedAttributeCount: int64(l.DroppedAttributeCount),
		}
	}
	return pl
}

func toProtoEvents(events []sdk.Event) []*protos.Span_Event {
	if len(events) == 0 {
		return nil
	}
	pe := make([]*protos.Span_Event, len(events))
	for i, e := range events {
		pe[i] = &protos.Span_Event{
			Name:                  e.Name,
			TimeMicros:            e.Time.UnixMicro(),
			Attributes:            toProtoAttrs(e.Attributes),
			DroppedAttributeCount: int64(e.DroppedAttributeCount),
		}
	}
	return pe
}

func toProtoStatus(s sdk.Status) *protos.Span_Status {
	ps := &protos.Span_Status{Error: s.Description}
	switch s.Code {
	case codes.Ok:
		ps.Code = protos.Span_Status_OK
	case codes.Error:
		ps.Code = protos.Span_Status_ERROR
	default:
		ps.Code = protos.Span_Status_UNSET
	}
	return ps
}

func toProtoScope(s instrumentation.Scope) *protos.Span_Scope {
	return &protos.Span_Scope{
		Name:      s.Name,
		Version:   s.Version,
		SchemaUrl: s.SchemaURL,
	}
}

func toProtoLibrary(l instrumentation.Library) *protos.Span_Library {
	return &protos.Span_Library{
		Name:      l.Name,
		Version:   l.Version,
		SchemaUrl: l.SchemaURL,
	}
}

func toProtoResource(r *resource.Resource) *protos.Span_Resource {
	if r == nil {
		return nil
	}
	return &protos.Span_Resource{
		SchemaUrl:  r.SchemaURL(),
		Attributes: toProtoAttrs(r.Attributes()),
	}
}
