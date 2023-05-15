// go:build !ignoreWeaverGen

package deploy

// Code generated by "weaver generate". DO NOT EDIT.
import (
	"context"
	"errors"
	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"reflect"
	"time"
)
var _ codegen.LatestVersion = codegen.Version[[0][10]struct{}]("You used 'weaver generate' codegen version 0.10.0, but you built your code with an incompatible weaver module version. Try upgrading 'weaver generate' and re-running it.")

func init() {
	codegen.Register(codegen.Registration{
		Name:  "github.com/ServiceWeaver/weaver/weavertest/internal/deploy/Started",
		Iface: reflect.TypeOf((*Started)(nil)).Elem(),
		Impl:  reflect.TypeOf(started{}),
		LocalStubFn: func(impl any, tracer trace.Tracer) any {
			return started_local_stub{impl: impl.(Started), tracer: tracer}
		},
		ClientStubFn: func(stub codegen.Stub, caller string) any {
			return started_client_stub{stub: stub, markStartedMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/weavertest/internal/deploy/Started", Method: "MarkStarted"})}
		},
		ServerStubFn: func(impl any, addLoad func(uint64, float64)) codegen.Server {
			return started_server_stub{impl: impl.(Started), addLoad: addLoad}
		},
		RefData: "",
	})
	codegen.Register(codegen.Registration{
		Name:        "github.com/ServiceWeaver/weaver/weavertest/internal/deploy/Widget",
		Iface:       reflect.TypeOf((*Widget)(nil)).Elem(),
		Impl:        reflect.TypeOf(widget{}),
		LocalStubFn: func(impl any, tracer trace.Tracer) any { return widget_local_stub{impl: impl.(Widget), tracer: tracer} },
		ClientStubFn: func(stub codegen.Stub, caller string) any {
			return widget_client_stub{stub: stub, useMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/weavertest/internal/deploy/Widget", Method: "Use"})}
		},
		ServerStubFn: func(impl any, addLoad func(uint64, float64)) codegen.Server {
			return widget_server_stub{impl: impl.(Widget), addLoad: addLoad}
		},
		RefData: "⟦f3fa3c18:wEaVeReDgE:github.com/ServiceWeaver/weaver/weavertest/internal/deploy/Widget→github.com/ServiceWeaver/weaver/weavertest/internal/deploy/Started⟧\n",
	})
}

// weaver.Instance checks.
var _ weaver.InstanceOf[Started] = &started{}
var _ weaver.InstanceOf[Widget] = &widget{}

// Local stub implementations.

type started_local_stub struct {
	impl   Started
	tracer trace.Tracer
}

// Check that started_local_stub implements the Started interface.
var _ Started = &started_local_stub{}

func (s started_local_stub) MarkStarted(ctx context.Context, a0 string) (err error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "deploy.Started.MarkStarted", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.MarkStarted(ctx, a0)
}

type widget_local_stub struct {
	impl   Widget
	tracer trace.Tracer
}

// Check that widget_local_stub implements the Widget interface.
var _ Widget = &widget_local_stub{}

func (s widget_local_stub) Use(ctx context.Context, a0 string) (err error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "deploy.Widget.Use", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.Use(ctx, a0)
}

// Client stub implementations.

type started_client_stub struct {
	stub               codegen.Stub
	markStartedMetrics *codegen.MethodMetrics
}

// Check that started_client_stub implements the Started interface.
var _ Started = &started_client_stub{}

func (s started_client_stub) MarkStarted(ctx context.Context, a0 string) (err error) {
	// Update metrics.
	start := time.Now()
	s.markStartedMetrics.Count.Add(1)

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "deploy.Started.MarkStarted", trace.WithSpanKind(trace.SpanKindClient))
	}

	defer func() {
		// Catch and return any panics detected during encoding/decoding/rpc.
		if err == nil {
			err = codegen.CatchPanics(recover())
			if err != nil {
				err = errors.Join(weaver.RemoteCallError, err)
			}
		}

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			s.markStartedMetrics.ErrorCount.Add(1)
		}
		span.End()

		s.markStartedMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
	}()

	// Preallocate a buffer of the right size.
	size := 0
	size += (4 + len(a0))
	enc := codegen.NewEncoder()
	enc.Reset(size)

	// Encode arguments.
	enc.String(a0)
	var shardKey uint64

	// Call the remote method.
	s.markStartedMetrics.BytesRequest.Put(float64(len(enc.Data())))
	var results []byte
	results, err = s.stub.Run(ctx, 0, enc.Data(), shardKey)
	if err != nil {
		err = errors.Join(weaver.RemoteCallError, err)
		return
	}
	s.markStartedMetrics.BytesReply.Put(float64(len(results)))

	// Decode the results.
	dec := codegen.NewDecoder(results)
	err = dec.Error()
	return
}

type widget_client_stub struct {
	stub       codegen.Stub
	useMetrics *codegen.MethodMetrics
}

// Check that widget_client_stub implements the Widget interface.
var _ Widget = &widget_client_stub{}

func (s widget_client_stub) Use(ctx context.Context, a0 string) (err error) {
	// Update metrics.
	start := time.Now()
	s.useMetrics.Count.Add(1)

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "deploy.Widget.Use", trace.WithSpanKind(trace.SpanKindClient))
	}

	defer func() {
		// Catch and return any panics detected during encoding/decoding/rpc.
		if err == nil {
			err = codegen.CatchPanics(recover())
			if err != nil {
				err = errors.Join(weaver.RemoteCallError, err)
			}
		}

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			s.useMetrics.ErrorCount.Add(1)
		}
		span.End()

		s.useMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
	}()

	// Preallocate a buffer of the right size.
	size := 0
	size += (4 + len(a0))
	enc := codegen.NewEncoder()
	enc.Reset(size)

	// Encode arguments.
	enc.String(a0)
	var shardKey uint64

	// Call the remote method.
	s.useMetrics.BytesRequest.Put(float64(len(enc.Data())))
	var results []byte
	results, err = s.stub.Run(ctx, 0, enc.Data(), shardKey)
	if err != nil {
		err = errors.Join(weaver.RemoteCallError, err)
		return
	}
	s.useMetrics.BytesReply.Put(float64(len(results)))

	// Decode the results.
	dec := codegen.NewDecoder(results)
	err = dec.Error()
	return
}

// Server stub implementations.

type started_server_stub struct {
	impl    Started
	addLoad func(key uint64, load float64)
}

// Check that started_server_stub implements the codegen.Server interface.
var _ codegen.Server = &started_server_stub{}

// GetStubFn implements the codegen.Server interface.
func (s started_server_stub) GetStubFn(method string) func(ctx context.Context, args []byte) ([]byte, error) {
	switch method {
	case "MarkStarted":
		return s.markStarted
	default:
		return nil
	}
}

func (s started_server_stub) markStarted(ctx context.Context, args []byte) (res []byte, err error) {
	// Catch and return any panics detected during encoding/decoding/rpc.
	defer func() {
		if err == nil {
			err = codegen.CatchPanics(recover())
		}
	}()

	// Decode arguments.
	dec := codegen.NewDecoder(args)
	var a0 string
	a0 = dec.String()

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	appErr := s.impl.MarkStarted(ctx, a0)

	// Encode the results.
	enc := codegen.NewEncoder()
	enc.Error(appErr)
	return enc.Data(), nil
}

type widget_server_stub struct {
	impl    Widget
	addLoad func(key uint64, load float64)
}

// Check that widget_server_stub implements the codegen.Server interface.
var _ codegen.Server = &widget_server_stub{}

// GetStubFn implements the codegen.Server interface.
func (s widget_server_stub) GetStubFn(method string) func(ctx context.Context, args []byte) ([]byte, error) {
	switch method {
	case "Use":
		return s.use
	default:
		return nil
	}
}

func (s widget_server_stub) use(ctx context.Context, args []byte) (res []byte, err error) {
	// Catch and return any panics detected during encoding/decoding/rpc.
	defer func() {
		if err == nil {
			err = codegen.CatchPanics(recover())
		}
	}()

	// Decode arguments.
	dec := codegen.NewDecoder(args)
	var a0 string
	a0 = dec.String()

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	appErr := s.impl.Use(ctx, a0)

	// Encode the results.
	enc := codegen.NewEncoder()
	enc.Error(appErr)
	return enc.Data(), nil
}
