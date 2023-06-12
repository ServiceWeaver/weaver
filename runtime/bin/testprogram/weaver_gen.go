//go:build !ignoreWeaverGen

package main

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
var _ codegen.LatestVersion = codegen.Version[[0][11]struct{}]("You used 'weaver generate' codegen version 0.11.0, but you built your code with an incompatible weaver module version. Try upgrading 'weaver generate' and re-running it.")

func init() {
	codegen.Register(codegen.Registration{
		Name:         "github.com/ServiceWeaver/weaver/runtime/bin/testprogram/A",
		Iface:        reflect.TypeOf((*A)(nil)).Elem(),
		Impl:         reflect.TypeOf(a{}),
		LocalStubFn:  func(impl any, tracer trace.Tracer) any { return a_local_stub{impl: impl.(A), tracer: tracer} },
		ClientStubFn: func(stub codegen.Stub, caller string) any { return a_client_stub{stub: stub} },
		ServerStubFn: func(impl any, addLoad func(uint64, float64)) codegen.Server {
			return a_server_stub{impl: impl.(A), addLoad: addLoad}
		},
		RefData: "⟦193f6c94:wEaVeReDgE:github.com/ServiceWeaver/weaver/runtime/bin/testprogram/A→github.com/ServiceWeaver/weaver/runtime/bin/testprogram/B⟧\n⟦8cd483a3:wEaVeReDgE:github.com/ServiceWeaver/weaver/runtime/bin/testprogram/A→github.com/ServiceWeaver/weaver/runtime/bin/testprogram/C⟧\n",
	})
	codegen.Register(codegen.Registration{
		Name:         "github.com/ServiceWeaver/weaver/runtime/bin/testprogram/B",
		Iface:        reflect.TypeOf((*B)(nil)).Elem(),
		Impl:         reflect.TypeOf(b{}),
		LocalStubFn:  func(impl any, tracer trace.Tracer) any { return b_local_stub{impl: impl.(B), tracer: tracer} },
		ClientStubFn: func(stub codegen.Stub, caller string) any { return b_client_stub{stub: stub} },
		ServerStubFn: func(impl any, addLoad func(uint64, float64)) codegen.Server {
			return b_server_stub{impl: impl.(B), addLoad: addLoad}
		},
		RefData: "",
	})
	codegen.Register(codegen.Registration{
		Name:         "github.com/ServiceWeaver/weaver/runtime/bin/testprogram/C",
		Iface:        reflect.TypeOf((*C)(nil)).Elem(),
		Impl:         reflect.TypeOf(c{}),
		LocalStubFn:  func(impl any, tracer trace.Tracer) any { return c_local_stub{impl: impl.(C), tracer: tracer} },
		ClientStubFn: func(stub codegen.Stub, caller string) any { return c_client_stub{stub: stub} },
		ServerStubFn: func(impl any, addLoad func(uint64, float64)) codegen.Server {
			return c_server_stub{impl: impl.(C), addLoad: addLoad}
		},
		RefData: "",
	})
	codegen.Register(codegen.Registration{
		Name:  "github.com/ServiceWeaver/weaver/Main",
		Iface: reflect.TypeOf((*weaver.Main)(nil)).Elem(),
		Impl:  reflect.TypeOf(app{}),
		LocalStubFn: func(impl any, tracer trace.Tracer) any {
			return main_local_stub{impl: impl.(weaver.Main), tracer: tracer}
		},
		ClientStubFn: func(stub codegen.Stub, caller string) any {
			return main_client_stub{stub: stub, mainMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/Main", Method: "Main"})}
		},
		ServerStubFn: func(impl any, addLoad func(uint64, float64)) codegen.Server {
			return main_server_stub{impl: impl.(weaver.Main), addLoad: addLoad}
		},
		RefData: "⟦d90475cb:wEaVeReDgE:github.com/ServiceWeaver/weaver/Main→github.com/ServiceWeaver/weaver/runtime/bin/testprogram/A⟧\n",
	})
}

// weaver.Instance checks.
var _ weaver.InstanceOf[A] = (*a)(nil)
var _ weaver.InstanceOf[B] = (*b)(nil)
var _ weaver.InstanceOf[C] = (*c)(nil)
var _ weaver.InstanceOf[weaver.Main] = (*app)(nil)

// weaver.Router checks.
var _ weaver.Unrouted = (*a)(nil)
var _ weaver.Unrouted = (*b)(nil)
var _ weaver.Unrouted = (*c)(nil)
var _ weaver.Unrouted = (*app)(nil)

// Local stub implementations.

type a_local_stub struct {
	impl   A
	tracer trace.Tracer
}

// Check that a_local_stub implements the A interface.
var _ A = (*a_local_stub)(nil)

type b_local_stub struct {
	impl   B
	tracer trace.Tracer
}

// Check that b_local_stub implements the B interface.
var _ B = (*b_local_stub)(nil)

type c_local_stub struct {
	impl   C
	tracer trace.Tracer
}

// Check that c_local_stub implements the C interface.
var _ C = (*c_local_stub)(nil)

type main_local_stub struct {
	impl   weaver.Main
	tracer trace.Tracer
}

// Check that main_local_stub implements the weaver.Main interface.
var _ weaver.Main = (*main_local_stub)(nil)

func (s main_local_stub) Main(ctx context.Context) (err error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "main.Main.Main", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.Main(ctx)
}

// Client stub implementations.

type a_client_stub struct {
	stub codegen.Stub
}

// Check that a_client_stub implements the A interface.
var _ A = (*a_client_stub)(nil)

type b_client_stub struct {
	stub codegen.Stub
}

// Check that b_client_stub implements the B interface.
var _ B = (*b_client_stub)(nil)

type c_client_stub struct {
	stub codegen.Stub
}

// Check that c_client_stub implements the C interface.
var _ C = (*c_client_stub)(nil)

type main_client_stub struct {
	stub        codegen.Stub
	mainMetrics *codegen.MethodMetrics
}

// Check that main_client_stub implements the weaver.Main interface.
var _ weaver.Main = (*main_client_stub)(nil)

func (s main_client_stub) Main(ctx context.Context) (err error) {
	// Update metrics.
	start := time.Now()
	s.mainMetrics.Count.Add(1)

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "main.Main.Main", trace.WithSpanKind(trace.SpanKindClient))
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
			s.mainMetrics.ErrorCount.Add(1)
		}
		span.End()

		s.mainMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
	}()

	var shardKey uint64

	// Call the remote method.
	s.mainMetrics.BytesRequest.Put(0)
	var results []byte
	results, err = s.stub.Run(ctx, 0, nil, shardKey)
	if err != nil {
		err = errors.Join(weaver.RemoteCallError, err)
		return
	}
	s.mainMetrics.BytesReply.Put(float64(len(results)))

	// Decode the results.
	dec := codegen.NewDecoder(results)
	err = dec.Error()
	return
}

// Server stub implementations.

type a_server_stub struct {
	impl    A
	addLoad func(key uint64, load float64)
}

// Check that a_server_stub implements the codegen.Server interface.
var _ codegen.Server = (*a_server_stub)(nil)

// GetStubFn implements the codegen.Server interface.
func (s a_server_stub) GetStubFn(method string) func(ctx context.Context, args []byte) ([]byte, error) {
	switch method {
	default:
		return nil
	}
}

type b_server_stub struct {
	impl    B
	addLoad func(key uint64, load float64)
}

// Check that b_server_stub implements the codegen.Server interface.
var _ codegen.Server = (*b_server_stub)(nil)

// GetStubFn implements the codegen.Server interface.
func (s b_server_stub) GetStubFn(method string) func(ctx context.Context, args []byte) ([]byte, error) {
	switch method {
	default:
		return nil
	}
}

type c_server_stub struct {
	impl    C
	addLoad func(key uint64, load float64)
}

// Check that c_server_stub implements the codegen.Server interface.
var _ codegen.Server = (*c_server_stub)(nil)

// GetStubFn implements the codegen.Server interface.
func (s c_server_stub) GetStubFn(method string) func(ctx context.Context, args []byte) ([]byte, error) {
	switch method {
	default:
		return nil
	}
}

type main_server_stub struct {
	impl    weaver.Main
	addLoad func(key uint64, load float64)
}

// Check that main_server_stub implements the codegen.Server interface.
var _ codegen.Server = (*main_server_stub)(nil)

// GetStubFn implements the codegen.Server interface.
func (s main_server_stub) GetStubFn(method string) func(ctx context.Context, args []byte) ([]byte, error) {
	switch method {
	case "Main":
		return s.main
	default:
		return nil
	}
}

func (s main_server_stub) main(ctx context.Context, args []byte) (res []byte, err error) {
	// Catch and return any panics detected during encoding/decoding/rpc.
	defer func() {
		if err == nil {
			err = codegen.CatchPanics(recover())
		}
	}()

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	appErr := s.impl.Main(ctx)

	// Encode the results.
	enc := codegen.NewEncoder()
	enc.Error(appErr)
	return enc.Data(), nil
}
