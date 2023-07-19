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

package traces

import (
	"math"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/protos"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// ReadSpan is a wrapper around *protos.Span that implements the
// Open Telemetry ReadOnlySpan interface.
type ReadSpan struct {
	sdk.ReadOnlySpan
	Span *protos.Span
}

var _ sdk.ReadOnlySpan = &ReadSpan{}

// Name implements the ReadOnlySpan interface.
func (s *ReadSpan) Name() string {
	return s.Span.Name
}

// SpanContext implements the ReadOnlySpan interface.
func (s *ReadSpan) SpanContext() trace.SpanContext {
	return fromProtoContext(s.Span.TraceId, s.Span.SpanId)
}

// Parent implements the ReadOnlySpan interface.
func (s *ReadSpan) Parent() trace.SpanContext {
	return fromProtoContext(s.Span.TraceId, s.Span.ParentSpanId)
}

// SpanKind implements the ReadOnlySpan interface.
func (s *ReadSpan) SpanKind() trace.SpanKind {
	return fromProtoKind(s.Span.Kind)
}

// StartTime implements the ReadOnlySpan interface.
func (s *ReadSpan) StartTime() time.Time {
	return time.UnixMicro(s.Span.StartMicros)
}

// EndTime implements the ReadOnlySpan interface.
func (s *ReadSpan) EndTime() time.Time {
	return time.UnixMicro(s.Span.EndMicros)
}

// Attributes implements the ReadOnlySpan interface.
func (s *ReadSpan) Attributes() []attribute.KeyValue {
	return fromProtoAttrs(s.Span.Attributes)
}

// Links implements the ReadOnlySpan interface.
func (s *ReadSpan) Links() []sdk.Link {
	return fromProtoLinks(s.Span.Links)
}

// Events implements the ReadOnlySpan interface.
func (s *ReadSpan) Events() []sdk.Event {
	return fromProtoEvents(s.Span.Events)
}

// Status implements the ReadOnlySpan interface.
func (s *ReadSpan) Status() sdk.Status {
	return fromProtoStatus(s.Span.Status)
}

// InstrumentationScope implements the ReadOnlySpan interface.
func (s *ReadSpan) InstrumentationScope() instrumentation.Scope {
	return fromProtoScope(s.Span.Scope)
}

// InstrumentationLibrary implements the ReadOnlySpan interface.
func (s *ReadSpan) InstrumentationLibrary() instrumentation.Scope {
	return fromProtoLibrary(s.Span.Library)
}

// Resource implements the ReadOnlySpan interface.
func (s *ReadSpan) Resource() *resource.Resource {
	return fromProtoResource(s.Span.Resource)
}

// DroppedAttributes implements the ReadOnlySpan interface.
func (s *ReadSpan) DroppedAttributes() int {
	return int(s.Span.DroppedAttributeCount)
}

// DroppedLinks implements the ReadOnlySpan interface.
func (s *ReadSpan) DroppedLinks() int {
	return int(s.Span.DroppedLinkCount)
}

// DroppedEvents implements the ReadOnlySpan interface.
func (s *ReadSpan) DroppedEvents() int {
	return int(s.Span.DroppedEventCount)
}

// ChildSpanCount implements the ReadOnlySpan interface.
func (s *ReadSpan) ChildSpanCount() int {
	return int(s.Span.ChildSpanCount)
}

func fromProtoContext(traceID, spanID []byte) trace.SpanContext {
	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: *(*trace.TraceID)(traceID),
		SpanID:  *(*trace.SpanID)(spanID),
	})
}

func fromProtoKind(kind protos.Span_Kind) trace.SpanKind {
	switch kind {
	case protos.Span_UNSPECIFIED:
		return trace.SpanKindUnspecified
	case protos.Span_INTERNAL:
		return trace.SpanKindInternal
	case protos.Span_SERVER:
		return trace.SpanKindServer
	case protos.Span_CLIENT:
		return trace.SpanKindClient
	case protos.Span_PRODUCER:
		return trace.SpanKindProducer
	case protos.Span_CONSUMER:
		return trace.SpanKindConsumer
	default:
		return trace.SpanKindInternal
	}
}

func fromProtoAttrs(attrs []*protos.Span_Attribute) []attribute.KeyValue {
	if attrs == nil {
		return nil
	}
	kvs := make([]attribute.KeyValue, len(attrs))
	for i, attr := range attrs {
		kv := attribute.KeyValue{Key: attribute.Key(attr.Key)}
		switch attr.Value.Type {
		case protos.Span_Attribute_Value_BOOL:
			val := false
			if attr.Value.GetNum() > 0 {
				val = true
			}
			kv.Value = attribute.BoolValue(val)
		case protos.Span_Attribute_Value_INT64:
			kv.Value = attribute.Int64Value(int64(attr.Value.GetNum()))
		case protos.Span_Attribute_Value_FLOAT64:
			kv.Value = attribute.Float64Value(math.Float64frombits(attr.Value.GetNum()))
		case protos.Span_Attribute_Value_STRING:
			kv.Value = attribute.StringValue(attr.Value.GetStr())
		case protos.Span_Attribute_Value_BOOLLIST:
			b := []byte(attr.Value.GetStr())
			vals := make([]bool, len(b))
			for i, v := range b {
				if v > 0 {
					vals[i] = true
				}
			}
			kv.Value = attribute.BoolSliceValue(vals)
		case protos.Span_Attribute_Value_INT64LIST:
			nums := attr.Value.GetNums().Nums
			vals := make([]int64, len(nums))
			for i, num := range nums {
				vals[i] = int64(num)
			}
			kv.Value = attribute.Int64SliceValue(vals)
		case protos.Span_Attribute_Value_FLOAT64LIST:
			nums := attr.Value.GetNums().Nums
			vals := make([]float64, len(nums))
			for i, num := range nums {
				vals[i] = math.Float64frombits(num)
			}
			kv.Value = attribute.Float64SliceValue(vals)
		case protos.Span_Attribute_Value_STRINGLIST:
			kv.Value = attribute.StringSliceValue(attr.Value.GetStrs().Strs)
		default:
			// kv.Value retains the default INVALID value.
		}
		kvs[i] = kv
	}
	return kvs
}

func fromProtoLinks(pl []*protos.Span_Link) []sdk.Link {
	if pl == nil {
		return nil
	}
	links := make([]sdk.Link, len(pl))
	for i, l := range pl {
		links[i] = sdk.Link{
			SpanContext:           fromProtoContext(l.TraceId, l.SpanId),
			Attributes:            fromProtoAttrs(l.Attributes),
			DroppedAttributeCount: int(l.DroppedAttributeCount),
		}
	}
	return links
}

func fromProtoEvents(pe []*protos.Span_Event) []sdk.Event {
	if pe == nil {
		return nil
	}
	events := make([]sdk.Event, len(pe))
	for i, e := range pe {
		events[i] = sdk.Event{
			Name:                  e.Name,
			Time:                  time.UnixMicro(e.TimeMicros),
			Attributes:            fromProtoAttrs(e.Attributes),
			DroppedAttributeCount: int(e.DroppedAttributeCount),
		}
	}
	return events
}

func fromProtoStatus(ps *protos.Span_Status) sdk.Status {
	if ps == nil {
		return sdk.Status{}
	}
	s := sdk.Status{Description: ps.Error}
	switch ps.Code {
	case protos.Span_Status_OK:
		s.Code = codes.Ok
	case protos.Span_Status_ERROR:
		s.Code = codes.Error
	default:
		s.Code = codes.Unset
	}
	return s
}

func fromProtoScope(ps *protos.Span_Scope) instrumentation.Scope {
	if ps == nil {
		return instrumentation.Scope{}
	}
	return instrumentation.Scope{
		Name:      ps.Name,
		Version:   ps.Version,
		SchemaURL: ps.SchemaUrl,
	}
}

func fromProtoLibrary(pl *protos.Span_Library) instrumentation.Scope {
	if pl == nil {
		return instrumentation.Scope{}
	}
	return instrumentation.Scope{
		Name:      pl.Name,
		Version:   pl.Version,
		SchemaURL: pl.SchemaUrl,
	}
}

func fromProtoResource(pr *protos.Span_Resource) *resource.Resource {
	if pr == nil {
		return nil
	}
	return resource.NewWithAttributes(pr.SchemaUrl, fromProtoAttrs(pr.Attributes)...)
}
