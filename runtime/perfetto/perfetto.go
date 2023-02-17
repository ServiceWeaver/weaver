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

// Package perfetto contains libraries for displaying trace information in the
// Perfetto UI and Chrome Tracer.
package perfetto

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
	"sync"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/logging"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	// Maps (colocation group, process id) to a readable replica number (e.g.,
	// Replica 1, Replica 2, etc.)
	mu   sync.Mutex
	pids map[string]map[int]int = map[string]map[int]int{}
)

// This file contains event tracing information to be rendered by the
// Perfetto UI [1] and Chrome tracer [3]
//
// The events are represented in JSON format as specified in [2]. This format
// is compatible with both Perfetto UI and Chrome tracer.
//
// [1] https://ui.perfetto.dev/
// [2] https://docs.google.com/document/d/1CvAClvFfyA5R-PhYUmn5OOQtYMH4h6I0nSsKchNAySU/preview#
// [3] https://sites.google.com/a/chromium.org/dev/developers/how-tos/trace-event-profiling-tool/frame-viewer
// [4] https://github.com/open-telemetry/opentelemetry-go/blob/main/sdk/trace/span.go

// completeEvent renders an event that contains a start time and a duration.
type completeEvent struct {
	Ph   string                       `json:"ph"`   // the event type
	Name string                       `json:"name"` // name of the event
	Cat  string                       `json:"cat"`  // category of the event
	Pid  int                          `json:"pid"`  // the id of the process that output the event
	Tid  int                          `json:"tid"`  // the thread id of the thread that output the event
	Ts   int64                        `json:"ts"`   // start time of the event
	Dur  int64                        `json:"dur"`  // duration of the event
	Args map[string]map[string]string `json:"args"` // arguments provided for the event
}

// metadataEvent renders an event that contains metadata information for a completeEvent.
type metadataEvent struct {
	Ph   string            `json:"ph"`   // the event type
	Name string            `json:"name"` // name of the event
	Cat  string            `json:"cat"`  // category of the event
	Pid  int               `json:"pid"`  // the id of the process that output the event
	Tid  int               `json:"tid"`  // the thread id of the thread that output the event
	Args map[string]string `json:"args"` // arguments provided for the event
}

// Encode converts a slice of span events into a JSON string that can be
// rendered by the Perfetto UI and Chrome Tracer.
//
// For each span, the following Perfetto events are generated:
//   - A complete event, which encapsulates the span duration.
//   - A number of metadata events, which correspond to span events.
func Encode(spans []sdktrace.ReadOnlySpan) ([]byte, error) {
	if len(spans) == 0 {
		return nil, nil
	}
	mu.Lock()
	defer mu.Unlock()
	var encoded []byte
	appendEvent := func(event any) error {
		b, err := json.Marshal(event)
		if err != nil {
			return err
		}
		encoded = append(encoded, b...)
		encoded = append(encoded, ',')
		return nil
	}

	for _, span := range spans {
		// Get the pid of the process that output the span, and the
		// corresponding colocation group.
		var pid int
		var colocGroup string
		for _, a := range span.Resource().Attributes() {
			if a.Key == semconv.ProcessPIDKey {
				pid = int(a.Value.AsInt64())
			} else if a.Key == "coloc_group" {
				colocGroup = a.Value.AsString()
			}
		}
		cg, found := pids[colocGroup]
		if !found {
			// Special entry to track the last replica id for this colocation
			// group.
			cg = map[int]int{0: 1}
			pids[colocGroup] = cg
		}
		replicaID, found := cg[pid]
		if !found {
			replicaID = cg[0]
			cg[pid] = replicaID
			cg[0]++
		}

		// The span name contains the name of the method that is called. Based
		// on the span kind, we attach an additional label to identify whether
		// the event happened at the client, server, or locally.
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

		// Generate a complete event and a series of metadata events.
		colocGroupFP := fp(colocGroup)

		// Build two metadata events for each colocation group (one to replace the process
		// name label and one for the thread name).
		if err := appendEvent(&metadataEvent{
			Ph:   "M", // make it a metadata event
			Name: "process_name",
			Cat:  span.SpanKind().String(),
			Pid:  colocGroupFP,
			Tid:  replicaID,
			Args: map[string]string{"name": fmt.Sprintf("%s+", logging.ShortenComponent(colocGroup))},
		}); err != nil {
			return nil, err
		}
		if err := appendEvent(&metadataEvent{
			Ph:   "M", // make it a metadata event
			Name: "thread_name",
			Cat:  span.SpanKind().String(),
			Pid:  colocGroupFP,
			Tid:  replicaID,
			Args: map[string]string{"name": "Replica"},
		}); err != nil {
			return nil, err
		}

		// Build a complete event.
		if err := appendEvent(&completeEvent{
			Ph:   "X", // make it a complete event
			Name: eventName,
			Cat:  span.SpanKind().String(),
			Pid:  colocGroupFP,
			Tid:  replicaID,
			Args: args,
			Ts:   span.StartTime().UnixMicro(),
			Dur:  span.EndTime().UnixMicro() - span.StartTime().UnixMicro(),
		}); err != nil {
			return nil, err
		}

		// For each span event, create a corresponding metadata event.
		for _, e := range span.Events() {
			attrs := map[string]string{}
			for _, a := range e.Attributes {
				attrs[string(a.Key)] = a.Value.Emit()
			}
			if err := appendEvent(&metadataEvent{
				Ph:   "M", // make it a metadata event
				Name: e.Name,
				Cat:  span.SpanKind().String(),
				Pid:  colocGroupFP,
				Tid:  replicaID,
				Args: attrs,
			}); err != nil {
				return nil, err
			}
		}
	}
	return encoded, nil
}

func fp(name string) int {
	hasher := sha256.New()
	hasher.Write([]byte(name))
	return int(binary.LittleEndian.Uint32(hasher.Sum(nil)))
}

// ServeLocalTraces runs an HTTP server that serves traces stored on the
// local file system.
//
// Perfetto UI requires that the server runs on the local port 9001. For that
// reason, this method will block until port 9001 becomes available. Because the
// server handles *all* traces stored on the local machine, as long as some
// other server has successfully started, local trace data will be served.
func ServeLocalTraces(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handler)
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

// handler is an HTTP handler that serves locally-stored trace files
// for trace requests from the Perfetto UI.
func handler(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(r.RequestURI, ".trace") {
		return
	}
	// Set access control according to:
	//   https://perfetto.dev/docs/visualization/deep-linking-to-perfetto-ui.
	w.Header().Set("Access-Control-Allow-Origin", "https://ui.perfetto.dev")
	data, err := os.ReadFile(r.RequestURI)
	if err != nil || len(data) == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	traces := make([]byte, len(data)+2)
	traces[0] = '['
	copy(traces[1:], data)
	traces[len(data)+1] = ']'
	w.Write(traces)
}
