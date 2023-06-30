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

package frontend

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type ctxKeyLogger struct{}
type ctxKeyRequestID struct{}
type ctxKeySessionID struct{}

type responseRecorder struct {
	b      int
	status int
	w      http.ResponseWriter
}

func (r *responseRecorder) Header() http.Header { return r.w.Header() }

func (r *responseRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.w.Write(p)
	r.b += n
	return n, err
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
	r.w.WriteHeader(statusCode)
}

type logHandler struct {
	server *Server
	next   http.Handler
}

func newLogHandler(server *Server, next http.Handler) http.Handler {
	return &logHandler{server: server, next: next}
}

func (lh *logHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID, _ := uuid.NewRandom()
	ctx = context.WithValue(ctx, ctxKeyRequestID{}, requestID.String())

	start := time.Now()
	rr := &responseRecorder{w: w}

	logger := lh.server.Logger(ctx).With(
		"http.req.path", r.URL.Path,
		"http.req.method", r.Method,
		"http.req.id", requestID)
	if v, ok := r.Context().Value(ctxKeySessionID{}).(string); ok {
		logger = logger.With("session", v)
	}
	logger.Debug("request started")
	defer func() {
		logger.Debug("request complete",
			"duration", time.Since(start),
			"status", rr.status,
			"bytes", rr.b,
		)
	}()

	ctx = context.WithValue(ctx, ctxKeyLogger{}, logger)
	r = r.WithContext(ctx)
	lh.next.ServeHTTP(rr, r)
}
