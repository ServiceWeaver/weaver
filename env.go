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

package weaver

import (
	"context"

	"github.com/ServiceWeaver/weaver/internal/logtype"
	"github.com/ServiceWeaver/weaver/internal/net/call"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// env provides the API by which a Service Weaver application process communicates with
// its execution environment, e.g., to do thing like starting processes etc.
type env interface {
	// GetWeaveletInfo returns the weavelet information from the environment.
	GetWeaveletInfo() *protos.WeaveletInfo

	// StartColocationGroup starts running the specified colocation group,
	// if it's not already started.
	// Succeeds without an error if it has already been started.
	StartColocationGroup(context.Context, *protos.ColocationGroup) error

	// RegisterComponentToStart registers a component to start in a given
	// target colocation group.
	//
	// The target colocation group periodically watches the set of registered
	// components and starts the components that haven't already been started.
	RegisterComponentToStart(ctx context.Context, targetGroup string,
		component string, isRouted bool) error

	// GetComponentsToStart returns the set of components that should be
	// started, given a version. It also returns the version that corresponds
	// to the set of components returned.
	GetComponentsToStart(ctx context.Context, version *call.Version) ([]string, *call.Version, error)

	// RegisterReplica registers this process's address. This allows other processes
	// in the application to make calls to components in this process.
	RegisterReplica(ctx context.Context, myAddress call.NetworkAddress) error

	// ReportLoad reports load information for a weavelet.
	ReportLoad(ctx context.Context, load *protos.WeaveletLoadReport) error

	// GetRoutingInfo returns the routing info for the provided colocation
	// group, including the set of replicas and the current routing assignments
	// (if any).
	GetRoutingInfo(ctx context.Context, group string, version *call.Version) (*protos.RoutingInfo, *call.Version, error)

	// GetAddress returns the address a weavelet should listen on for a
	// listener.
	GetAddress(ctx context.Context, listener string, opts ListenerOptions) (*protos.GetAddressReply, error)

	// ExportListener exports a listener.
	ExportListener(ctx context.Context, lis *protos.Listener, opts ListenerOptions) (*protos.ExportListenerReply, error)

	// CreateLogSaver creates and returns a function that saves log entries
	// to the environment.
	CreateLogSaver(ctx context.Context, component string) func(entry *protos.LogEntry)

	// SystemLogger returns the Logger for system messages.
	SystemLogger() logtype.Logger

	// CreateTraceExporter returns an exporter that should be used for
	// exporting trace spans. A nil exporter value means that no traces
	// should be exported.
	CreateTraceExporter() (sdktrace.SpanExporter, error)
}

// getEnv returns the env to use for this weavelet.
func getEnv(ctx context.Context) (env, error) {
	bootstrap, err := runtime.GetBootstrap(ctx)
	if err != nil {
		return nil, err
	}
	if !bootstrap.HasPipes() {
		return newSingleprocessEnv(bootstrap)
	}
	return newRemoteEnv(ctx, bootstrap)
}
