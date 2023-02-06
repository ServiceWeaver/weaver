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
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"github.com/ServiceWeaver/weaver/runtime/logging"
)

// This file contains event tracing information to be rendered by the Perfetto UI[1].
//
// The events are represented in JSON format as specified in [2], format compatible
// both with Perfetto UI and Chrome tracing[3].
//
// Each event contains information exported by an OTEL read span[4]. There are
// multiple types of JSON events[2]. However, for rendering read only spans, we
// picked two types of events:
// * Complete events - they are duration events but in a more compact format
// * Metadata events - to associate extra information, in our case read span events
//
// [1] https://ui.perfetto.dev/
// [2] https://docs.google.com/document/d/1CvAClvFfyA5R-PhYUmn5OOQtYMH4h6I0nSsKchNAySU/preview#
// [3] https://sites.google.com/a/chromium.org/dev/developers/how-tos/trace-event-profiling-tool/frame-viewer
// [4] https://github.com/open-telemetry/opentelemetry-go/blob/main/sdk/trace/span.go

// CompleteEvent renders an event that contains a start time and a duration.
type CompleteEvent struct {
	Ph   string                       `json:"ph"`   // the event type
	Name string                       `json:"name"` // name of the event
	Cat  string                       `json:"cat"`  // category of the event
	Pid  int                          `json:"pid"`  // the id of the process that output the event
	Tid  int                          `json:"tid"`  // the thread id of the thread that output the event
	Ts   int64                        `json:"ts"`   // start time of the event
	Dur  int64                        `json:"dur"`  // duration of the event
	Args map[string]map[string]string `json:"args"` // arguments provided for the event
}

// MetadataEvent renders an event that contains metadata information for a CompleteEvent.
type MetadataEvent struct {
	Ph   string            `json:"ph"`   // the event type
	Name string            `json:"name"` // name of the event
	Cat  string            `json:"cat"`  // category of the event
	Pid  int               `json:"pid"`  // the id of the process that output the event
	Tid  int               `json:"tid"`  // the thread id of the thread that output the event
	Args map[string]string `json:"args"` // arguments provided for the event
}

// ChromeTraceEncoder encodes read spans into JSON events that can be rendered by
// Chrome tracer and Perfetto UI.
type ChromeTraceEncoder struct {
	encoded []byte // encoded traces in JSON format

	// Needed to map colocation group process ids to more readable numbers (e.g.,
	// Replica 1, Replica 2, etc.)
	mapping map[string]map[int]int // keyed by colocation group and process id
}

func NewChromeTraceEncoder() *ChromeTraceEncoder {
	return &ChromeTraceEncoder{mapping: map[string]map[int]int{}}
}

// Encode converts a slice of read span events to events that can be rendered by
// chrome tracer and Perfetto UI.
//
// For each span event we generate the following:
//   - a complete event for each colocation group and corresponding metadata events to label
//     the process/replica name
//   - a metadata event for each event in the span, encapsulated in a context event
func (c *ChromeTraceEncoder) Encode(spans []sdktrace.ReadOnlySpan) []byte {
	if len(spans) == 0 {
		return nil
	}
	c.encoded = c.encoded[:0] // clear any previous state

	for _, span := range spans {
		// Get the pid of the process that output the span, and the corresponding colocation group.
		var pid int
		var colocGroup string
		for _, a := range span.Resource().Attributes() {
			if a.Key == semconv.ProcessPIDKey {
				pid = int(a.Value.AsInt64())
			} else if a.Key == "coloc_group" {
				colocGroup = a.Value.AsString()
			}
		}
		if _, found := c.mapping[colocGroup]; !found {
			c.mapping[colocGroup] = map[int]int{}
			c.mapping[colocGroup][0] = 1 // special entry to track the last replica id for this colocation group
		}
		if _, found := c.mapping[colocGroup][pid]; !found {
			c.mapping[colocGroup][pid] = c.mapping[colocGroup][0]
			c.mapping[colocGroup][0]++
		}
		replicaId := c.mapping[colocGroup][pid]

		// The span name contains the name of the method that is called. Based on the
		// span kind, we attach an additional label to identify whether the event
		// happened at the client, server, or local.
		eventName := span.Name()
		switch span.SpanKind() {
		case trace.SpanKindServer:
			eventName = eventName + " [server]"
		case trace.SpanKindClient:
			eventName = eventName + " [client]"
		case trace.SpanKindInternal:
			eventName = eventName + " [local]"
		}

		// Build the attributes map.
		attrs := map[string]string{}
		for _, a := range span.Attributes() {
			attrs[string(a.Key)] = a.Value.Emit()
		}

		// Build the arguments.
		args := map[string]map[string]string{
			"ids": {
				"spanID":        span.SpanContext().SpanID().String(),
				"traceID":       span.SpanContext().TraceID().String(),
				"parentSpanID":  span.Parent().SpanID().String(),
				"parentTraceID": span.Parent().TraceID().String(),
				"processID":     strconv.Itoa(pid),
			},
			"attributes": attrs,
		}

		// Generate a complete event, and a series of metadata events.
		colocGroupFP := fp(colocGroup)

		// Build two metadata events for each colocation group (one to replace the process
		// name label and one for the thread name).
		c.encodeEvent(&MetadataEvent{
			Ph:   "M", // make it a metadata event
			Name: "process_name",
			Cat:  span.SpanKind().String(),
			Pid:  colocGroupFP,
			Tid:  replicaId,
			Args: map[string]string{"name": fmt.Sprintf("%s+", logging.ShortenComponent(colocGroup))},
		})
		c.encodeEvent(&MetadataEvent{
			Ph:   "M", // make it a metadata event
			Name: "thread_name",
			Cat:  span.SpanKind().String(),
			Pid:  colocGroupFP,
			Tid:  replicaId,
			Args: map[string]string{"name": "Replica"},
		})

		// Build a complete event.
		c.encodeEvent(&CompleteEvent{
			Ph:   "X", // make it a complete event
			Name: eventName,
			Cat:  span.SpanKind().String(),
			Pid:  colocGroupFP,
			Tid:  replicaId,
			Args: args,
			Ts:   span.StartTime().UnixMicro(),
			Dur:  span.EndTime().UnixMicro() - span.StartTime().UnixMicro(),
		})

		if len(span.Events()) == 0 {
			continue
		}

		// For each span event, create a corresponding metadata event.
		for _, e := range span.Events() {
			attrs := map[string]string{}
			for _, a := range e.Attributes {
				attrs[string(a.Key)] = a.Value.Emit()
			}
			c.encodeEvent(&MetadataEvent{
				Ph:   "M", // make it a metadata event
				Name: e.Name,
				Cat:  span.SpanKind().String(),
				Pid:  colocGroupFP,
				Tid:  replicaId,
				Args: attrs,
			})
		}
	}
	return c.encoded
}

func (c *ChromeTraceEncoder) encodeEvent(ev any) {
	b, _ := json.Marshal(ev)
	c.encoded = append(c.encoded, b...)
	c.encoded = append(c.encoded, []byte(",")...)
}

func fp(name string) int {
	hasher := sha256.New()
	hasher.Write([]byte(name))
	return int(binary.LittleEndian.Uint32(hasher.Sum(nil)))
}

// RunTracerProvider runs an HTTP server that handles trace requests for all the
// active deployments.
//
// Periodically, it attempts to listen on port 9001, if not able to listen yet.
// I.e., among all active deployments on a single machine, only a single instance
// will run the trace provider server. Because trace files are stored on disk, any
// active server will serve all the files, so if this server can't start, some
// other server will be able to serve its file.
//
// It has to listen on port 9001 because it is the only port from which Perfetto UI
// can read the trace file.
func RunTracerProvider(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", tracePage)
	server := http.Server{Handler: mux}

	ticker := time.NewTicker(time.Second)
	for {
		select {
		case <-ticker.C:
			lis, err := net.Listen("tcp", "localhost:9001")
			if err == nil {
				ticker.Stop()
				server.Serve(lis)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func tracePage(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.RequestURI, ".trace") {
		// Set access control according to https://perfetto.dev/docs/visualization/deep-linking-to-perfetto-ui.
		w.Header().Set("Access-Control-Allow-Origin", "https://ui.perfetto.dev")
		data, _ := os.ReadFile(r.RequestURI)
		if len(data) == 0 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		traces := make([]byte, len(data)+2)
		traces[0] = '['
		copy(traces[1:], data)
		traces[len(data)+1] = ']'
		w.Write(traces)
	}
}
