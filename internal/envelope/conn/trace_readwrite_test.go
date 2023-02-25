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
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	sdk "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/protobuf/testing/protocmp"
)

type pipeForTest struct {
	envelopeConn *conn.EnvelopeConn
	wletConn     *conn.WeaveletConn

	waitToExportSpans sync.WaitGroup // wait for trace spans to be exported
	spans             []sdk.ReadOnlySpan
}

var _ conn.EnvelopeHandler = &pipeForTest{}

func (p *pipeForTest) RecvTraceSpans(spans []sdk.ReadOnlySpan) error {
	p.spans = spans
	p.waitToExportSpans.Done()
	return nil
}

func (p *pipeForTest) RecvLogEntry(*protos.LogEntry)                      {}
func (p *pipeForTest) StartComponent(*protos.ComponentToStart) error      { return nil }
func (p *pipeForTest) RegisterReplica(*protos.ReplicaToRegister) error    { return nil }
func (p *pipeForTest) StartColocationGroup(*protos.ColocationGroup) error { return nil }
func (p *pipeForTest) ReportLoad(*protos.WeaveletLoadReport) error        { return nil }
func (p *pipeForTest) GetRoutingInfo(*protos.GetRoutingInfo) (*protos.RoutingInfo, error) {
	return nil, nil
}
func (p *pipeForTest) GetComponentsToStart(*protos.GetComponentsToStart) (*protos.ComponentsToStart, error) {
	return nil, nil
}
func (p *pipeForTest) GetAddress(*protos.GetAddressRequest) (*protos.GetAddressReply, error) {
	return nil, nil
}
func (p *pipeForTest) ExportListener(*protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
	return nil, nil
}

func writeAndRead(in sdk.ReadOnlySpan, pipe *pipeForTest) (sdk.ReadOnlySpan, error) {
	pipe.waitToExportSpans.Add(1)
	writer := traceio.NewWriter(pipe.wletConn.SendTraceSpans)
	if err := writer.ExportSpans(context.Background(), []sdk.ReadOnlySpan{in}); err != nil {
		return nil, err
	}
	pipe.waitToExportSpans.Wait()

	if len(pipe.spans) != 1 {
		panic(fmt.Sprintf("too many spans: want 1, got %d", len(pipe.spans)))
	}
	return pipe.spans[0], nil
}

func testAttrs() []*protos.Attribute {
	num := func(t protos.Attribute_Value_Type, v uint64) *protos.Attribute_Value {
		return &protos.Attribute_Value{
			Type:  t,
			Value: &protos.Attribute_Value_Num{Num: v},
		}
	}
	str := func(t protos.Attribute_Value_Type, s string) *protos.Attribute_Value {
		return &protos.Attribute_Value{
			Type:  t,
			Value: &protos.Attribute_Value_Str{Str: s},
		}
	}
	nums := func(t protos.Attribute_Value_Type, v ...uint64) *protos.Attribute_Value {
		return &protos.Attribute_Value{
			Type: t,
			Value: &protos.Attribute_Value_Nums{
				Nums: &protos.Attribute_Value_NumberList{Nums: v},
			},
		}
	}
	strs := func(t protos.Attribute_Value_Type, s ...string) *protos.Attribute_Value {
		return &protos.Attribute_Value{
			Type: t,
			Value: &protos.Attribute_Value_Strs{
				Strs: &protos.Attribute_Value_StringList{Strs: s},
			},
		}
	}
	bools := func(vals ...bool) *protos.Attribute_Value {
		b := make([]byte, len(vals))
		for i, v := range vals {
			if v {
				b[i] = 1
			}
		}
		return str(protos.Attribute_Value_BOOLLIST, string(b))
	}
	return []*protos.Attribute{
		{
			Key: "invalid",
			Value: &protos.Attribute_Value{
				Type: protos.Attribute_Value_INVALID,
			},
		},
		{
			Key:   "false",
			Value: num(protos.Attribute_Value_BOOL, 0),
		},
		{
			Key:   "true",
			Value: num(protos.Attribute_Value_BOOL, 1),
		},
		{
			Key:   "zero int",
			Value: num(protos.Attribute_Value_INT64, 0),
		},
		{
			Key:   "non-zero int",
			Value: num(protos.Attribute_Value_INT64, 99),
		},
		{
			Key:   "zero float",
			Value: num(protos.Attribute_Value_FLOAT64, math.Float64bits(0.0)),
		},
		{
			Key:   "non-zero float",
			Value: num(protos.Attribute_Value_FLOAT64, math.Float64bits(99.9)),
		},
		{
			Key:   "empty string",
			Value: str(protos.Attribute_Value_STRING, ""),
		},
		{
			Key:   "non-empty string",
			Value: str(protos.Attribute_Value_STRING, "serviceweaver"),
		},
		{
			Key:   "empty bool slice",
			Value: bools(),
		},
		{
			Key:   "non-empty bool slice",
			Value: bools(true, false, true),
		},
		{
			Key:   "empty int slice",
			Value: nums(protos.Attribute_Value_INT64LIST),
		},
		{
			Key:   "non-empty int slice",
			Value: nums(protos.Attribute_Value_INT64LIST, 0, 9, 99),
		},
		{
			Key:   "empty float slice",
			Value: nums(protos.Attribute_Value_FLOAT64LIST),
		},
		{
			Key: "non-empty float slice",
			Value: nums(protos.Attribute_Value_FLOAT64LIST,
				math.Float64bits(9.9),
				math.Float64bits(0.0),
				math.Float64bits(99.9)),
		},
		{
			Key:   "empty string slice",
			Value: strs(protos.Attribute_Value_STRINGLIST),
		},
		{
			Key: "non-empty string slice",
			Value: strs(protos.Attribute_Value_STRINGLIST,
				"serviceweaver",
				"",
				"serviceweaver"),
		},
	}
}

func TestTracesReadWrite(t *testing.T) {
	rnd := func() []byte {
		b := uuid.New()
		return b[:]
	}
	tid := rnd()[:16]
	now := time.Now()
	expect := &protos.Span{
		Name:         "span",
		TraceId:      tid[:],
		SpanId:       rnd()[:8],
		ParentSpanId: rnd()[:8],
		Kind:         protos.SpanKind_CONSUMER,
		StartMicros:  now.UnixMicro(),
		EndMicros:    now.Add(1 * time.Second).UnixMicro(),
		Attributes:   testAttrs(),
		Links: []*protos.Span_Link{
			{
				TraceId:               rnd()[:16],
				SpanId:                rnd()[:8],
				Attributes:            testAttrs(),
				DroppedAttributeCount: 5,
			},
			{
				TraceId:               rnd()[:16],
				SpanId:                rnd()[:8],
				Attributes:            testAttrs(),
				DroppedAttributeCount: 7,
			},
		},
		Events: []*protos.Span_Event{
			{
				Name:                  "serviceweaver",
				Attributes:            testAttrs(),
				DroppedAttributeCount: 5,
				TimeMicros:            time.Now().UnixMicro(),
			},
			{
				Name:                  "serviceweavers",
				Attributes:            testAttrs(),
				DroppedAttributeCount: 7,
				TimeMicros:            time.Now().UnixMicro(),
			},
		},
		Status: &protos.Span_Status{
			Code:  protos.Span_Status_ERROR,
			Error: "serviceweaver",
		},
		Library: &protos.Span_Library{
			Name:      "serviceweaver",
			Version:   "v2",
			SchemaUrl: "serviceweaver://service.weaver",
		},
		Resource: &protos.Span_Resource{
			SchemaUrl: "serviceweaver://service.weaver",
			// Remove the invalid attribute, which cannot be passed into the
			// trace.Resource type.
			Attributes: testAttrs()[1:],
		},
		DroppedAttributeCount: 5,
		DroppedLinkCount:      6,
		DroppedEventCount:     7,
		ChildSpanCount:        8,
	}

	pipe := &pipeForTest{}
	pipe.envelopeConn, pipe.wletConn = makeConnections(t, pipe)
	msg, err := writeAndRead(&traceio.ReadSpan{Span: expect}, pipe)
	if err != nil {
		t.Fatal(err)
	}
	actual, ok := msg.(*traceio.ReadSpan)
	if !ok {
		t.Fatalf("invalid message type: want *protos.ReadSpan, got %T", msg)
	}
	if diff := cmp.Diff(expect,
		actual.Span,
		protocmp.Transform(),
		protocmp.SortRepeatedFields((*protos.Span)(nil), "attributes"),
		protocmp.SortRepeatedFields((*protos.Span_Link)(nil), "attributes"),
		protocmp.SortRepeatedFields((*protos.Span_Event)(nil), "attributes"),
		protocmp.SortRepeatedFields((*protos.Span_Resource)(nil), "attributes"),
	); diff != "" {
		t.Fatalf("span: (-want,+got):\n%s\n", diff)
	}
}
