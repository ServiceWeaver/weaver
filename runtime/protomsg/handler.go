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

package protomsg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/ServiceWeaver/weaver/metrics"
	"google.golang.org/protobuf/proto"
)

var (
	httpRequestCounts = metrics.NewCounterMap[handlerLabels](
		"serviceweaver_system_http_request_count",
		"Count of Service Weaver HTTP requests received",
	)
	httpRequestErrorCounts = metrics.NewCounterMap[errorLabels](
		"serviceweaver_system_http_request_error_count",
		"Count of Service Weaver HTTP requests received that result in an error",
	)
	httpRequestLatencyMicros = metrics.NewHistogramMap[handlerLabels](
		"serviceweaver_system_http_request_latency_micros",
		"Duration, in microseconds, of Service Weaver HTTP request execution",
		metrics.NonNegativeBuckets,
	)
	httpRequestBytesReceived = metrics.NewHistogramMap[handlerLabels](
		"serviceweaver_system_http_request_bytes_received",
		"Number of bytes received by Service Weaver HTTP request handlers",
		metrics.NonNegativeBuckets,
	)
	httpRequestBytesReturned = metrics.NewHistogramMap[handlerLabels](
		"serviceweaver_system_http_request_bytes_returned",
		"Number of bytes returned by Service Weaver HTTP request handlers",
		metrics.NonNegativeBuckets,
	)
)

type handlerLabels struct {
	Path string // HTTP request URL path (e.g., "/manager/start_process")
}

type errorLabels struct {
	// HTTP request URL path (e.g., "/manager/start_process").
	Path string

	// The type of error. An HTTP request can fail in five places:
	//
	//   1. Reading the request.
	//   2. Unmarshaling the request.
	//   3. Executing the request.
	//   4. Marshaling the response.
	//   5. Writing the response.
	Error string
}

// ProtoPointer[T] is an interface which asserts that *T is a proto.Message.
// See [1] for an overview of this idiom.
//
// [1]: https://go.googlesource.com/proposal/+/refs/heads/master/design/43651-type-parameters.md#pointer-method-example
type ProtoPointer[T any] interface {
	*T
	proto.Message
}

// HandlerFunc converts a protobuf based handler into an http.HandlerFunc. The
// returned http.HandlerFunc automatically unmarshals an *I from the HTTP
// request and invokes the provided handler. If the handler successfully
// returns an *O, it is marshaled into the body of the HTTP response.
// Otherwise, the returned error is logged and returned in the HTTP response.
// The context passed to the handler is the HTTP request's context.
func HandlerFunc[I, O any, IP ProtoPointer[I], OP ProtoPointer[O]](logger *slog.Logger, handler func(context.Context, *I) (*O, error)) http.HandlerFunc {
	f := func(w http.ResponseWriter, r *http.Request) {
		var in I
		if err := fromHTTP(w, r, IP(&in)); err != nil {
			logger.Error("parse http request", "err", err, "method", r.Method, "url", r.URL)
			return
		}
		out, err := handler(r.Context(), &in)
		if err != nil {
			httpRequestErrorCounts.Get(errorLabels{r.URL.Path, "execute request"}).Add(1.0)
			logger.Error("handle http RPC", "err", err, "method", r.Method, "url", r.URL)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		} else {
			toHTTP(w, r, OP(out))
		}
	}
	return metricHandler(panicHandler(logger, f))
}

// HandlerThunk converts a protobuf based handler into an http.HandlerFunc. If
// the handler successfully returns an *O, it is marshaled into the body of the
// HTTP response. Otherwise, the returned error is logged and returned in the
// HTTP response. The context passed to the handler is the HTTP request's
// context.
func HandlerThunk[O any, OP ProtoPointer[O]](logger *slog.Logger, handler func(context.Context) (*O, error)) http.HandlerFunc {
	f := func(w http.ResponseWriter, r *http.Request) {
		out, err := handler(r.Context())
		if err != nil {
			httpRequestErrorCounts.Get(errorLabels{r.URL.Path, "execute request"}).Add(1.0)
			logger.Error("handle http RPC", "err", err, "method", r.Method, "url", r.URL)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		} else {
			toHTTP(w, r, OP(out))
		}
	}
	return metricHandler(panicHandler(logger, f))
}

// HandlerDo converts a protobuf based handler into an http.HandlerFunc. The
// returned http.HandlerFunc automatically unmarshals an *I from the HTTP
// request and invokes the provided handler. Errors are logged and returned in
// the HTTP response. The context passed to the handler is the HTTP request's
// context.
func HandlerDo[I any, IP ProtoPointer[I]](logger *slog.Logger, handler func(context.Context, *I) error) http.HandlerFunc {
	f := func(w http.ResponseWriter, r *http.Request) {
		var in I
		if err := fromHTTP(w, r, IP(&in)); err != nil {
			logger.Error("parse http request", "err", err, "method", r.Method, "url", r.URL)
			return
		}
		if err := handler(r.Context(), &in); err != nil {
			httpRequestErrorCounts.Get(errorLabels{r.URL.Path, "execute request"}).Add(1.0)
			logger.Error("handle http RPC", "err", err, "method", r.Method, "url", r.URL)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	return metricHandler(panicHandler(logger, f))
}

// panicHandler wraps the provided handler in a new handler which catches and
// returns panics as 500 "Internal Server Error" responses.
func panicHandler(logger *slog.Logger, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				err := fmt.Errorf("%s:\n%v", err, string(debug.Stack()))
				logger.Error("panic in HTTP handler", "err", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}()
		handler(w, r)
	}
}

// metricHandler wraps the provided handler in a new handler which records
// basic metrics about the handler's execution (e.g., execution time).
func metricHandler(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		labels := handlerLabels{r.URL.Path}
		httpRequestCounts.Get(labels).Add(1)
		defer func() {
			duration := float64(time.Since(start).Microseconds())
			httpRequestLatencyMicros.Get(labels).Put(duration)
		}()
		handler(w, r)
	}
}

// toHTTP writes msgs into a given response.  On error, it writes the
// error into the response instead.
func toHTTP(w http.ResponseWriter, r *http.Request, msgs ...proto.Message) {
	out, err := toWire(msgs...)
	if err != nil {
		httpRequestErrorCounts.Get(errorLabels{r.URL.Path, "marshal response"}).Add(1.0)
		msg := fmt.Sprintf("cannot marshal response protos: %v", err)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	httpRequestBytesReturned.Get(handlerLabels{r.URL.Path}).Put(float64(len(out)))
	if _, err = w.Write(out); err != nil {
		httpRequestErrorCounts.Get(errorLabels{r.URL.Path, "write response"}).Add(1.0)
		msg := fmt.Sprintf("cannot write responses: %v", err)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
}

// fromHTTP fills msgs from a given request body.
// If successful, it returns nil and leaves the response writer unmodified;
// otherwise, it returns an error and sets the error status on the response.
func fromHTTP(w http.ResponseWriter, r *http.Request, msgs ...proto.Message) error {
	in, err := io.ReadAll(r.Body)
	if err != nil {
		httpRequestErrorCounts.Get(errorLabels{r.URL.Path, "read request"}).Add(1.0)
		msg := "cannot read request body"
		http.Error(w, msg, http.StatusBadRequest)
		return errors.New(msg)
	}
	httpRequestBytesReceived.Get(handlerLabels{r.URL.Path}).Put(float64(len(in)))
	if err := fromWire(in, msgs...); err != nil {
		httpRequestErrorCounts.Get(errorLabels{r.URL.Path, "unmarshal request"}).Add(1.0)
		msg := fmt.Sprintf("cannot unmarshal request protos from %q: %v", in, err)
		http.Error(w, msg, http.StatusBadRequest)
		return errors.New(msg)
	}
	return nil
}
