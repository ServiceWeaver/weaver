// go:build !ignoreWeaverGen

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
		Name:        "github.com/ServiceWeaver/weaver/examples/collatz/Even",
		Iface:       reflect.TypeOf((*Even)(nil)).Elem(),
		Impl:        reflect.TypeOf(even{}),
		LocalStubFn: func(impl any, tracer trace.Tracer) any { return even_local_stub{impl: impl.(Even), tracer: tracer} },
		ClientStubFn: func(stub codegen.Stub, caller string) any {
			return even_client_stub{stub: stub, doMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/collatz/Even", Method: "Do"})}
		},
		ServerStubFn: func(impl any, addLoad func(uint64, float64)) codegen.Server {
			return even_server_stub{impl: impl.(Even), addLoad: addLoad}
		},
		RefData: "",
	})
	codegen.Register(codegen.Registration{
		Name:  "github.com/ServiceWeaver/weaver/Main",
		Iface: reflect.TypeOf((*weaver.Main)(nil)).Elem(),
		Impl:  reflect.TypeOf(server{}),
		LocalStubFn: func(impl any, tracer trace.Tracer) any {
			return main_local_stub{impl: impl.(weaver.Main), tracer: tracer}
		},
		ClientStubFn: func(stub codegen.Stub, caller string) any {
			return main_client_stub{stub: stub, mainMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/Main", Method: "Main"})}
		},
		ServerStubFn: func(impl any, addLoad func(uint64, float64)) codegen.Server {
			return main_server_stub{impl: impl.(weaver.Main), addLoad: addLoad}
		},
		RefData: "⟦f95ad2dd:wEaVeReDgE:github.com/ServiceWeaver/weaver/Main→github.com/ServiceWeaver/weaver/examples/collatz/Odd⟧\n⟦987c175b:wEaVeReDgE:github.com/ServiceWeaver/weaver/Main→github.com/ServiceWeaver/weaver/examples/collatz/Even⟧\n⟦f3b62957:wEaVeRlIsTeNeRs:github.com/ServiceWeaver/weaver/Main→collatz⟧\n",
	})
	codegen.Register(codegen.Registration{
		Name:        "github.com/ServiceWeaver/weaver/examples/collatz/Odd",
		Iface:       reflect.TypeOf((*Odd)(nil)).Elem(),
		Impl:        reflect.TypeOf(odd{}),
		LocalStubFn: func(impl any, tracer trace.Tracer) any { return odd_local_stub{impl: impl.(Odd), tracer: tracer} },
		ClientStubFn: func(stub codegen.Stub, caller string) any {
			return odd_client_stub{stub: stub, doMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/collatz/Odd", Method: "Do"})}
		},
		ServerStubFn: func(impl any, addLoad func(uint64, float64)) codegen.Server {
			return odd_server_stub{impl: impl.(Odd), addLoad: addLoad}
		},
		RefData: "",
	})
}

// weaver.Instance checks.
var _ weaver.InstanceOf[Even] = (*even)(nil)
var _ weaver.InstanceOf[weaver.Main] = (*server)(nil)
var _ weaver.InstanceOf[Odd] = (*odd)(nil)

// weaver.Router checks.
var _ weaver.Unrouted = (*even)(nil)
var _ weaver.Unrouted = (*server)(nil)
var _ weaver.Unrouted = (*odd)(nil)

// Local stub implementations.

type even_local_stub struct {
	impl   Even
	tracer trace.Tracer
}

// Check that even_local_stub implements the Even interface.
var _ Even = (*even_local_stub)(nil)

func (s even_local_stub) Do(ctx context.Context, a0 int) (r0 int, err error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "main.Even.Do", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.Do(ctx, a0)
}

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

type odd_local_stub struct {
	impl   Odd
	tracer trace.Tracer
}

// Check that odd_local_stub implements the Odd interface.
var _ Odd = (*odd_local_stub)(nil)

func (s odd_local_stub) Do(ctx context.Context, a0 int) (r0 int, err error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "main.Odd.Do", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.Do(ctx, a0)
}

// Client stub implementations.

type even_client_stub struct {
	stub      codegen.Stub
	doMetrics *codegen.MethodMetrics
}

// Check that even_client_stub implements the Even interface.
var _ Even = (*even_client_stub)(nil)

func (s even_client_stub) Do(ctx context.Context, a0 int) (r0 int, err error) {
	// Update metrics.
	start := time.Now()
	s.doMetrics.Count.Add(1)

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "main.Even.Do", trace.WithSpanKind(trace.SpanKindClient))
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
			s.doMetrics.ErrorCount.Add(1)
		}
		span.End()

		s.doMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
	}()

	// Preallocate a buffer of the right size.
	size := 0
	size += 8
	enc := codegen.NewEncoder()
	enc.Reset(size)

	// Encode arguments.
	enc.Int(a0)
	var shardKey uint64

	// Call the remote method.
	s.doMetrics.BytesRequest.Put(float64(len(enc.Data())))
	var results []byte
	results, err = s.stub.Run(ctx, 0, enc.Data(), shardKey)
	if err != nil {
		err = errors.Join(weaver.RemoteCallError, err)
		return
	}
	s.doMetrics.BytesReply.Put(float64(len(results)))

	// Decode the results.
	dec := codegen.NewDecoder(results)
	r0 = dec.Int()
	err = dec.Error()
	return
}

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

type odd_client_stub struct {
	stub      codegen.Stub
	doMetrics *codegen.MethodMetrics
}

// Check that odd_client_stub implements the Odd interface.
var _ Odd = (*odd_client_stub)(nil)

func (s odd_client_stub) Do(ctx context.Context, a0 int) (r0 int, err error) {
	// Update metrics.
	start := time.Now()
	s.doMetrics.Count.Add(1)

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "main.Odd.Do", trace.WithSpanKind(trace.SpanKindClient))
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
			s.doMetrics.ErrorCount.Add(1)
		}
		span.End()

		s.doMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
	}()

	// Preallocate a buffer of the right size.
	size := 0
	size += 8
	enc := codegen.NewEncoder()
	enc.Reset(size)

	// Encode arguments.
	enc.Int(a0)
	var shardKey uint64

	// Call the remote method.
	s.doMetrics.BytesRequest.Put(float64(len(enc.Data())))
	var results []byte
	results, err = s.stub.Run(ctx, 0, enc.Data(), shardKey)
	if err != nil {
		err = errors.Join(weaver.RemoteCallError, err)
		return
	}
	s.doMetrics.BytesReply.Put(float64(len(results)))

	// Decode the results.
	dec := codegen.NewDecoder(results)
	r0 = dec.Int()
	err = dec.Error()
	return
}

// Server stub implementations.

type even_server_stub struct {
	impl    Even
	addLoad func(key uint64, load float64)
}

// Check that even_server_stub implements the codegen.Server interface.
var _ codegen.Server = (*even_server_stub)(nil)

// GetStubFn implements the codegen.Server interface.
func (s even_server_stub) GetStubFn(method string) func(ctx context.Context, args []byte) ([]byte, error) {
	switch method {
	case "Do":
		return s.do
	default:
		return nil
	}
}

func (s even_server_stub) do(ctx context.Context, args []byte) (res []byte, err error) {
	// Catch and return any panics detected during encoding/decoding/rpc.
	defer func() {
		if err == nil {
			err = codegen.CatchPanics(recover())
		}
	}()

	// Decode arguments.
	dec := codegen.NewDecoder(args)
	var a0 int
	a0 = dec.Int()

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	r0, appErr := s.impl.Do(ctx, a0)

	// Encode the results.
	enc := codegen.NewEncoder()
	enc.Int(r0)
	enc.Error(appErr)
	return enc.Data(), nil
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

type odd_server_stub struct {
	impl    Odd
	addLoad func(key uint64, load float64)
}

// Check that odd_server_stub implements the codegen.Server interface.
var _ codegen.Server = (*odd_server_stub)(nil)

// GetStubFn implements the codegen.Server interface.
func (s odd_server_stub) GetStubFn(method string) func(ctx context.Context, args []byte) ([]byte, error) {
	switch method {
	case "Do":
		return s.do
	default:
		return nil
	}
}

func (s odd_server_stub) do(ctx context.Context, args []byte) (res []byte, err error) {
	// Catch and return any panics detected during encoding/decoding/rpc.
	defer func() {
		if err == nil {
			err = codegen.CatchPanics(recover())
		}
	}()

	// Decode arguments.
	dec := codegen.NewDecoder(args)
	var a0 int
	a0 = dec.Int()

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	r0, appErr := s.impl.Do(ctx, a0)

	// Encode the results.
	enc := codegen.NewEncoder()
	enc.Int(r0)
	enc.Error(appErr)
	return enc.Data(), nil
}
