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

package call

import (
	"log/slog"
	"time"

	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"go.opentelemetry.io/otel/trace"
)

// ClientOptions are the options to configure an RPC client.
type ClientOptions struct {
	// Load balancer. Defaults to RoundRobin() if nil.
	Balancer Balancer

	// Logger. Defaults to a logger that logs to stderr.
	Logger *slog.Logger

	// If non-zero, each call will optimistically spin for a given duration
	// before blocking, waiting for the results.
	OptimisticSpinDuration time.Duration

	// If non-zero, all writes smaller than this limit are flattened into
	// a single buffer before being written on the connection.
	WriteFlattenLimit int
}

// ServerOption are the options to configure an RPC server.
type ServerOptions struct {
	// Logger. Defaults to a logger that logs to stderr.
	Logger *slog.Logger

	// Tracer. Defaults to a discarding tracer.
	Tracer trace.Tracer

	// If non-zero, calls on the server are inlined and a new goroutine is
	// launched only if the call takes longer than the provided duration.
	InlineHandlerDuration time.Duration

	// If non-zero, all writes smaller than this limit are flattened into
	// a single buffer before being written on the connection.
	WriteFlattenLimit int
}

// CallOptions are call-specific options.
type CallOptions struct {
	// Retry indicates whether or not calls that failed due to communication
	// errors should be retried.
	Retry bool

	// ShardKey, if not 0, is the shard key that a Balancer can use to route a
	// call. A Balancer can always choose to ignore the ShardKey.
	//
	// TODO(mwhittaker): Figure out a way to have 0 be a valid shard key. Could
	// change to *uint64 for example.
	ShardKey uint64
}

// withDefaults returns a copy of the ClientOptions with zero values replaced
// with default values.
func (c ClientOptions) withDefaults() ClientOptions {
	if c.Logger == nil {
		c.Logger = logging.StderrLogger(logging.Options{})
	}
	if c.Balancer == nil {
		c.Balancer = RoundRobin()
	}
	return c
}

// withDefaults returns a copy of the ServerOptions with zero values replaced
// with default values.
func (s ServerOptions) withDefaults() ServerOptions {
	if s.Logger == nil {
		s.Logger = logging.StderrLogger(logging.Options{})
	}
	if s.Tracer == nil {
		s.Tracer = traceio.TestTracer()
	}
	return s
}
