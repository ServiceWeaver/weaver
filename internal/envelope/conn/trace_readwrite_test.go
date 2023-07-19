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

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/internal/envelope/conn"
	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"google.golang.org/protobuf/testing/protocmp"
)

type pipeForTest struct {
	envelopeConn *conn.EnvelopeConn
	wletConn     *conn.WeaveletConn

	waitToExportSpans sync.WaitGroup // wait for trace spans to be exported
	spans             *protos.TraceSpans
}

var _ conn.EnvelopeHandler = &pipeForTest{}

func (p *pipeForTest) HandleTraceSpans(_ context.Context, spans *protos.TraceSpans) error {
	p.spans = spans
	p.waitToExportSpans.Done()
	return nil
}

func (*pipeForTest) HandleLogEntry(context.Context, *protos.LogEntry) error {
	return nil
}

func (*pipeForTest) ActivateComponent(context.Context, *protos.ActivateComponentRequest) (*protos.ActivateComponentReply, error) {
	return nil, nil
}

func (*pipeForTest) GetListenerAddress(context.Context, *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error) {
	return nil, nil
}

func (*pipeForTest) ExportListener(context.Context, *protos.ExportListenerRequest) (*protos.ExportListenerReply, error) {
	return nil, nil
}

func (*pipeForTest) GetSelfCertificate(context.Context, *protos.GetSelfCertificateRequest) (*protos.GetSelfCertificateReply, error) {
	panic("unused")
}

func (*pipeForTest) VerifyClientCertificate(context.Context, *protos.VerifyClientCertificateRequest) (*protos.VerifyClientCertificateReply, error) {
	panic("unused")
}

func (*pipeForTest) VerifyServerCertificate(context.Context, *protos.VerifyServerCertificateRequest) (*protos.VerifyServerCertificateReply, error) {
	panic("unused")
}

func writeAndRead(in *protos.Span, pipe *pipeForTest) (*protos.Span, error) {
	pipe.waitToExportSpans.Add(1)
	writer := traceio.NewWriter(pipe.wletConn.SendTraceSpans)
	if err := writer.ExportSpansProto(&protos.TraceSpans{Span: []*protos.Span{in}}); err != nil {
		return nil, err
	}
	pipe.waitToExportSpans.Wait()

	if len(pipe.spans.Span) != 1 {
		panic(fmt.Sprintf("too many spans: want 1, got %d", len(pipe.spans.Span)))
	}
	return pipe.spans.Span[0], nil
}

func testAttrs() []*protos.Span_Attribute {
	num := func(t protos.Span_Attribute_Value_Type, v uint64) *protos.Span_Attribute_Value {
		return &protos.Span_Attribute_Value{
			Type:  t,
			Value: &protos.Span_Attribute_Value_Num{Num: v},
		}
	}
	str := func(t protos.Span_Attribute_Value_Type, s string) *protos.Span_Attribute_Value {
		return &protos.Span_Attribute_Value{
			Type:  t,
			Value: &protos.Span_Attribute_Value_Str{Str: s},
		}
	}
	nums := func(t protos.Span_Attribute_Value_Type, v ...uint64) *protos.Span_Attribute_Value {
		return &protos.Span_Attribute_Value{
			Type: t,
			Value: &protos.Span_Attribute_Value_Nums{
				Nums: &protos.Span_Attribute_Value_NumberList{Nums: v},
			},
		}
	}
	strs := func(t protos.Span_Attribute_Value_Type, s ...string) *protos.Span_Attribute_Value {
		return &protos.Span_Attribute_Value{
			Type: t,
			Value: &protos.Span_Attribute_Value_Strs{
				Strs: &protos.Span_Attribute_Value_StringList{Strs: s},
			},
		}
	}
	bools := func(vals ...bool) *protos.Span_Attribute_Value {
		b := make([]byte, len(vals))
		for i, v := range vals {
			if v {
				b[i] = 1
			}
		}
		return str(protos.Span_Attribute_Value_BOOLLIST, string(b))
	}
	return []*protos.Span_Attribute{
		{
			Key: "invalid",
			Value: &protos.Span_Attribute_Value{
				Type: protos.Span_Attribute_Value_INVALID,
			},
		},
		{
			Key:   "false",
			Value: num(protos.Span_Attribute_Value_BOOL, 0),
		},
		{
			Key:   "true",
			Value: num(protos.Span_Attribute_Value_BOOL, 1),
		},
		{
			Key:   "zero int",
			Value: num(protos.Span_Attribute_Value_INT64, 0),
		},
		{
			Key:   "non-zero int",
			Value: num(protos.Span_Attribute_Value_INT64, 99),
		},
		{
			Key:   "zero float",
			Value: num(protos.Span_Attribute_Value_FLOAT64, math.Float64bits(0.0)),
		},
		{
			Key:   "non-zero float",
			Value: num(protos.Span_Attribute_Value_FLOAT64, math.Float64bits(99.9)),
		},
		{
			Key:   "empty string",
			Value: str(protos.Span_Attribute_Value_STRING, ""),
		},
		{
			Key:   "non-empty string",
			Value: str(protos.Span_Attribute_Value_STRING, "serviceweaver"),
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
			Value: nums(protos.Span_Attribute_Value_INT64LIST),
		},
		{
			Key:   "non-empty int slice",
			Value: nums(protos.Span_Attribute_Value_INT64LIST, 0, 9, 99),
		},
		{
			Key:   "empty float slice",
			Value: nums(protos.Span_Attribute_Value_FLOAT64LIST),
		},
		{
			Key: "non-empty float slice",
			Value: nums(protos.Span_Attribute_Value_FLOAT64LIST,
				math.Float64bits(9.9),
				math.Float64bits(0.0),
				math.Float64bits(99.9)),
		},
		{
			Key:   "empty string slice",
			Value: strs(protos.Span_Attribute_Value_STRINGLIST),
		},
		{
			Key: "non-empty string slice",
			Value: strs(protos.Span_Attribute_Value_STRINGLIST,
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
		Kind:         protos.Span_CONSUMER,
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
		Scope: &protos.Span_Scope{
			Name:      "serviceweaver",
			Version:   "v2",
			SchemaUrl: "serviceweaver://service.weaver",
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
	actual, err := writeAndRead(expect, pipe)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(expect,
		actual,
		protocmp.Transform(),
		protocmp.SortRepeatedFields((*protos.Span)(nil), "attributes"),
		protocmp.SortRepeatedFields((*protos.Span_Link)(nil), "attributes"),
		protocmp.SortRepeatedFields((*protos.Span_Event)(nil), "attributes"),
		protocmp.SortRepeatedFields((*protos.Span_Resource)(nil), "attributes"),
	); diff != "" {
		t.Fatalf("span: (-want,+got):\n%s\n", diff)
	}
}

// Register a dummy component for test.
func init() {
	type A interface{}

	type aimpl struct {
		weaver.Implements[A]
	}

	register[A, aimpl]("conn_test/A")
}
