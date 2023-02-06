package simple

// Code generated by "weaver generate". DO NOT EDIT.
import (
	"context"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"reflect"
	"time"
)

func init() {
	codegen.Register(codegen.Registration{
		Name:   "github.com/ServiceWeaver/weaver/weavertest/internal/simple/Destination",
		Iface:  reflect.TypeOf((*Destination)(nil)).Elem(),
		New:    func() any { return &destination{} },
		Routed: true,
		LocalStubFn: func(impl any, tracer trace.Tracer) any {
			return destination_local_stub{impl: impl.(Destination), tracer: tracer}
		},
		ClientStubFn: func(stub codegen.Stub, caller string) any {
			return destination_client_stub{stub: stub, getpidMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/weavertest/internal/simple/Destination", Method: "Getpid"}), recordMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/weavertest/internal/simple/Destination", Method: "Record"}), getAllMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/weavertest/internal/simple/Destination", Method: "GetAll"}), routedRecordMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/weavertest/internal/simple/Destination", Method: "RoutedRecord"})}
		},
		ServerStubFn: func(impl any, addLoad func(uint64, float64)) codegen.Server {
			return destination_server_stub{impl: impl.(Destination), addLoad: addLoad}
		},
	})
	codegen.Register(codegen.Registration{
		Name:        "github.com/ServiceWeaver/weaver/weavertest/internal/simple/Source",
		Iface:       reflect.TypeOf((*Source)(nil)).Elem(),
		New:         func() any { return &source{} },
		LocalStubFn: func(impl any, tracer trace.Tracer) any { return source_local_stub{impl: impl.(Source), tracer: tracer} },
		ClientStubFn: func(stub codegen.Stub, caller string) any {
			return source_client_stub{stub: stub, emitMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/weavertest/internal/simple/Source", Method: "Emit"})}
		},
		ServerStubFn: func(impl any, addLoad func(uint64, float64)) codegen.Server {
			return source_server_stub{impl: impl.(Source), addLoad: addLoad}
		},
	})
}

// Local stub implementations.

type destination_local_stub struct {
	impl   Destination
	tracer trace.Tracer
}

func (s destination_local_stub) Getpid(ctx context.Context) (r0 int, err error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "simple.Destination.Getpid", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.Getpid(ctx)
}

func (s destination_local_stub) Record(ctx context.Context, a0 string, a1 string) (err error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "simple.Destination.Record", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.Record(ctx, a0, a1)
}

func (s destination_local_stub) GetAll(ctx context.Context, a0 string) (r0 []string, err error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "simple.Destination.GetAll", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.GetAll(ctx, a0)
}

func (s destination_local_stub) RoutedRecord(ctx context.Context, a0 string, a1 string) (err error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "simple.Destination.RoutedRecord", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.RoutedRecord(ctx, a0, a1)
}

type source_local_stub struct {
	impl   Source
	tracer trace.Tracer
}

func (s source_local_stub) Emit(ctx context.Context, a0 string, a1 string) (err error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "simple.Source.Emit", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.Emit(ctx, a0, a1)
}

// Client stub implementations.

type destination_client_stub struct {
	stub                codegen.Stub
	getpidMetrics       *codegen.MethodMetrics
	recordMetrics       *codegen.MethodMetrics
	getAllMetrics       *codegen.MethodMetrics
	routedRecordMetrics *codegen.MethodMetrics
}

func (s destination_client_stub) Getpid(ctx context.Context) (r0 int, err error) {
	// Update metrics.
	start := time.Now()
	s.getpidMetrics.Count.Add(1)

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "simple.Destination.Getpid", trace.WithSpanKind(trace.SpanKindClient))
	}

	defer func() {
		// Catch and return any panics detected during encoding/decoding/rpc.
		if err == nil {
			err = codegen.CatchPanics(recover())
		}
		err = s.stub.WrapError(err)

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			s.getpidMetrics.ErrorCount.Add(1)
		}
		span.End()

		s.getpidMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
	}()

	var shardKey uint64

	// Call the remote method.
	s.getpidMetrics.BytesRequest.Put(0)
	var results []byte
	results, err = s.stub.Run(ctx, 1, nil, shardKey)
	if err != nil {
		return
	}
	s.getpidMetrics.BytesReply.Put(float64(len(results)))

	// Decode the results.
	dec := codegen.NewDecoder(results)
	r0 = dec.Int()
	err = dec.Error()
	return
}

func (s destination_client_stub) Record(ctx context.Context, a0 string, a1 string) (err error) {
	// Update metrics.
	start := time.Now()
	s.recordMetrics.Count.Add(1)

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "simple.Destination.Record", trace.WithSpanKind(trace.SpanKindClient))
	}

	defer func() {
		// Catch and return any panics detected during encoding/decoding/rpc.
		if err == nil {
			err = codegen.CatchPanics(recover())
		}
		err = s.stub.WrapError(err)

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			s.recordMetrics.ErrorCount.Add(1)
		}
		span.End()

		s.recordMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
	}()

	// Preallocate a buffer of the right size.
	size := 0
	size += (4 + len(a0))
	size += (4 + len(a1))
	enc := codegen.NewEncoder()
	enc.Reset(size)

	// Encode arguments.
	enc.String(a0)
	enc.String(a1)
	var shardKey uint64

	// Call the remote method.
	s.recordMetrics.BytesRequest.Put(float64(len(enc.Data())))
	var results []byte
	results, err = s.stub.Run(ctx, 2, enc.Data(), shardKey)
	if err != nil {
		return
	}
	s.recordMetrics.BytesReply.Put(float64(len(results)))

	// Decode the results.
	dec := codegen.NewDecoder(results)
	err = dec.Error()
	return
}

func (s destination_client_stub) GetAll(ctx context.Context, a0 string) (r0 []string, err error) {
	// Update metrics.
	start := time.Now()
	s.getAllMetrics.Count.Add(1)

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "simple.Destination.GetAll", trace.WithSpanKind(trace.SpanKindClient))
	}

	defer func() {
		// Catch and return any panics detected during encoding/decoding/rpc.
		if err == nil {
			err = codegen.CatchPanics(recover())
		}
		err = s.stub.WrapError(err)

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			s.getAllMetrics.ErrorCount.Add(1)
		}
		span.End()

		s.getAllMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
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
	s.getAllMetrics.BytesRequest.Put(float64(len(enc.Data())))
	var results []byte
	results, err = s.stub.Run(ctx, 0, enc.Data(), shardKey)
	if err != nil {
		return
	}
	s.getAllMetrics.BytesReply.Put(float64(len(results)))

	// Decode the results.
	dec := codegen.NewDecoder(results)
	r0 = serviceweaver_dec_slice_string_4af10117(dec)
	err = dec.Error()
	return
}

func (s destination_client_stub) RoutedRecord(ctx context.Context, a0 string, a1 string) (err error) {
	// Update metrics.
	start := time.Now()
	s.routedRecordMetrics.Count.Add(1)

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "simple.Destination.RoutedRecord", trace.WithSpanKind(trace.SpanKindClient))
	}

	defer func() {
		// Catch and return any panics detected during encoding/decoding/rpc.
		if err == nil {
			err = codegen.CatchPanics(recover())
		}
		err = s.stub.WrapError(err)

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			s.routedRecordMetrics.ErrorCount.Add(1)
		}
		span.End()

		s.routedRecordMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
	}()

	// Preallocate a buffer of the right size.
	size := 0
	size += (4 + len(a0))
	size += (4 + len(a1))
	enc := codegen.NewEncoder()
	enc.Reset(size)

	// Encode arguments.
	enc.String(a0)
	enc.String(a1)

	// Set the shardKey.
	var r destRouter
	shardKey := _hashDestination(r.RoutedRecord(ctx, a0, a1))

	// Call the remote method.
	s.routedRecordMetrics.BytesRequest.Put(float64(len(enc.Data())))
	var results []byte
	results, err = s.stub.Run(ctx, 3, enc.Data(), shardKey)
	if err != nil {
		return
	}
	s.routedRecordMetrics.BytesReply.Put(float64(len(results)))

	// Decode the results.
	dec := codegen.NewDecoder(results)
	err = dec.Error()
	return
}

type source_client_stub struct {
	stub        codegen.Stub
	emitMetrics *codegen.MethodMetrics
}

func (s source_client_stub) Emit(ctx context.Context, a0 string, a1 string) (err error) {
	// Update metrics.
	start := time.Now()
	s.emitMetrics.Count.Add(1)

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "simple.Source.Emit", trace.WithSpanKind(trace.SpanKindClient))
	}

	defer func() {
		// Catch and return any panics detected during encoding/decoding/rpc.
		if err == nil {
			err = codegen.CatchPanics(recover())
		}
		err = s.stub.WrapError(err)

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			s.emitMetrics.ErrorCount.Add(1)
		}
		span.End()

		s.emitMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
	}()

	// Preallocate a buffer of the right size.
	size := 0
	size += (4 + len(a0))
	size += (4 + len(a1))
	enc := codegen.NewEncoder()
	enc.Reset(size)

	// Encode arguments.
	enc.String(a0)
	enc.String(a1)
	var shardKey uint64

	// Call the remote method.
	s.emitMetrics.BytesRequest.Put(float64(len(enc.Data())))
	var results []byte
	results, err = s.stub.Run(ctx, 0, enc.Data(), shardKey)
	if err != nil {
		return
	}
	s.emitMetrics.BytesReply.Put(float64(len(results)))

	// Decode the results.
	dec := codegen.NewDecoder(results)
	err = dec.Error()
	return
}

// Server stub implementations.

type destination_server_stub struct {
	impl    Destination
	addLoad func(key uint64, load float64)
}

// GetStubFn implements the stub.Server interface.
func (s destination_server_stub) GetStubFn(method string) func(ctx context.Context, args []byte) ([]byte, error) {
	switch method {
	case "Getpid":
		return s.getpid
	case "Record":
		return s.record
	case "GetAll":
		return s.getAll
	case "RoutedRecord":
		return s.routedRecord
	default:
		return nil
	}
}

func (s destination_server_stub) getpid(ctx context.Context, args []byte) (res []byte, err error) {
	// Catch and return any panics detected during encoding/decoding/rpc.
	defer func() {
		if err == nil {
			err = codegen.CatchPanics(recover())
		}
	}()

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	r0, appErr := s.impl.Getpid(ctx)

	// Encode the results.
	enc := codegen.NewEncoder()
	enc.Int(r0)
	enc.Error(appErr)
	return enc.Data(), nil
}

func (s destination_server_stub) record(ctx context.Context, args []byte) (res []byte, err error) {
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
	var a1 string
	a1 = dec.String()

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	appErr := s.impl.Record(ctx, a0, a1)

	// Encode the results.
	enc := codegen.NewEncoder()
	enc.Error(appErr)
	return enc.Data(), nil
}

func (s destination_server_stub) getAll(ctx context.Context, args []byte) (res []byte, err error) {
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
	r0, appErr := s.impl.GetAll(ctx, a0)

	// Encode the results.
	enc := codegen.NewEncoder()
	serviceweaver_enc_slice_string_4af10117(enc, r0)
	enc.Error(appErr)
	return enc.Data(), nil
}

func (s destination_server_stub) routedRecord(ctx context.Context, args []byte) (res []byte, err error) {
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
	var a1 string
	a1 = dec.String()
	var r destRouter
	s.addLoad(_hashDestination(r.RoutedRecord(ctx, a0, a1)), 1.0)

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	appErr := s.impl.RoutedRecord(ctx, a0, a1)

	// Encode the results.
	enc := codegen.NewEncoder()
	enc.Error(appErr)
	return enc.Data(), nil
}

type source_server_stub struct {
	impl    Source
	addLoad func(key uint64, load float64)
}

// GetStubFn implements the stub.Server interface.
func (s source_server_stub) GetStubFn(method string) func(ctx context.Context, args []byte) ([]byte, error) {
	switch method {
	case "Emit":
		return s.emit
	default:
		return nil
	}
}

func (s source_server_stub) emit(ctx context.Context, args []byte) (res []byte, err error) {
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
	var a1 string
	a1 = dec.String()

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	appErr := s.impl.Emit(ctx, a0, a1)

	// Encode the results.
	enc := codegen.NewEncoder()
	enc.Error(appErr)
	return enc.Data(), nil
}

// Router methods.

// _hashDestination returns a 64 bit hash of the provided value.
func _hashDestination(r string) uint64 {
	var h codegen.Hasher
	h.WriteString(string(r))
	return h.Sum64()
}

// _orderedCodeDestination returns an order-preserving serialization of the provided value.
func _orderedCodeDestination(r string) codegen.OrderedCode {
	var enc codegen.OrderedEncoder
	enc.WriteString(string(r))
	return enc.Encode()
}

// Encoding/decoding implementations.

func serviceweaver_enc_slice_string_4af10117(enc *codegen.Encoder, arg []string) {
	if arg == nil {
		enc.Len(-1)
		return
	}
	enc.Len(len(arg))
	for i := 0; i < len(arg); i++ {
		enc.String(arg[i])
	}
}

func serviceweaver_dec_slice_string_4af10117(dec *codegen.Decoder) []string {
	n := dec.Len()
	if n == -1 {
		return nil
	}
	res := make([]string, n)
	for i := 0; i < n; i++ {
		res[i] = dec.String()
	}
	return res
}
