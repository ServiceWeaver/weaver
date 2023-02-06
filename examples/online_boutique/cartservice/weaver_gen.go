package cartservice

// Code generated by "weaver generate". DO NOT EDIT.
import (
	"context"
	"fmt"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"reflect"
	"time"
)

func init() {
	codegen.Register(codegen.Registration{
		Name:        "github.com/ServiceWeaver/weaver/examples/online_boutique/cartservice/T",
		Iface:       reflect.TypeOf((*T)(nil)).Elem(),
		New:         func() any { return &impl{} },
		LocalStubFn: func(impl any, tracer trace.Tracer) any { return t_local_stub{impl: impl.(T), tracer: tracer} },
		ClientStubFn: func(stub codegen.Stub, caller string) any {
			return t_client_stub{stub: stub, addItemMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/online_boutique/cartservice/T", Method: "AddItem"}), getCartMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/online_boutique/cartservice/T", Method: "GetCart"}), emptyCartMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/online_boutique/cartservice/T", Method: "EmptyCart"})}
		},
		ServerStubFn: func(impl any, addLoad func(uint64, float64)) codegen.Server {
			return t_server_stub{impl: impl.(T), addLoad: addLoad}
		},
	})
	codegen.Register(codegen.Registration{
		Name:   "github.com/ServiceWeaver/weaver/examples/online_boutique/cartservice/cartCache",
		Iface:  reflect.TypeOf((*cartCache)(nil)).Elem(),
		New:    func() any { return &cartCacheImpl{} },
		Routed: true,
		LocalStubFn: func(impl any, tracer trace.Tracer) any {
			return cartCache_local_stub{impl: impl.(cartCache), tracer: tracer}
		},
		ClientStubFn: func(stub codegen.Stub, caller string) any {
			return cartCache_client_stub{stub: stub, addMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/online_boutique/cartservice/cartCache", Method: "Add"}), getMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/online_boutique/cartservice/cartCache", Method: "Get"}), removeMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/online_boutique/cartservice/cartCache", Method: "Remove"})}
		},
		ServerStubFn: func(impl any, addLoad func(uint64, float64)) codegen.Server {
			return cartCache_server_stub{impl: impl.(cartCache), addLoad: addLoad}
		},
	})
}

// Local stub implementations.

type t_local_stub struct {
	impl   T
	tracer trace.Tracer
}

func (s t_local_stub) AddItem(ctx context.Context, a0 string, a1 CartItem) (err error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "cartservice.T.AddItem", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.AddItem(ctx, a0, a1)
}

func (s t_local_stub) GetCart(ctx context.Context, a0 string) (r0 []CartItem, err error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "cartservice.T.GetCart", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.GetCart(ctx, a0)
}

func (s t_local_stub) EmptyCart(ctx context.Context, a0 string) (err error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "cartservice.T.EmptyCart", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.EmptyCart(ctx, a0)
}

type cartCache_local_stub struct {
	impl   cartCache
	tracer trace.Tracer
}

func (s cartCache_local_stub) Add(ctx context.Context, a0 string, a1 []CartItem) (err error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "cartservice.cartCache.Add", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.Add(ctx, a0, a1)
}

func (s cartCache_local_stub) Get(ctx context.Context, a0 string) (r0 []CartItem, err error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "cartservice.cartCache.Get", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.Get(ctx, a0)
}

func (s cartCache_local_stub) Remove(ctx context.Context, a0 string) (r0 bool, err error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "cartservice.cartCache.Remove", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.Remove(ctx, a0)
}

// Client stub implementations.

type t_client_stub struct {
	stub             codegen.Stub
	addItemMetrics   *codegen.MethodMetrics
	getCartMetrics   *codegen.MethodMetrics
	emptyCartMetrics *codegen.MethodMetrics
}

func (s t_client_stub) AddItem(ctx context.Context, a0 string, a1 CartItem) (err error) {
	// Update metrics.
	start := time.Now()
	s.addItemMetrics.Count.Add(1)

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "cartservice.T.AddItem", trace.WithSpanKind(trace.SpanKindClient))
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
			s.addItemMetrics.ErrorCount.Add(1)
		}
		span.End()

		s.addItemMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
	}()

	// Preallocate a buffer of the right size.
	size := 0
	size += (4 + len(a0))
	size += serviceweaver_size_CartItem_f4da907b(&a1)
	enc := codegen.NewEncoder()
	enc.Reset(size)

	// Encode arguments.
	enc.String(a0)
	(a1).WeaverMarshal(enc)
	var shardKey uint64

	// Call the remote method.
	s.addItemMetrics.BytesRequest.Put(float64(len(enc.Data())))
	var results []byte
	results, err = s.stub.Run(ctx, 0, enc.Data(), shardKey)
	if err != nil {
		return
	}
	s.addItemMetrics.BytesReply.Put(float64(len(results)))

	// Decode the results.
	dec := codegen.NewDecoder(results)
	err = dec.Error()
	return
}

func (s t_client_stub) GetCart(ctx context.Context, a0 string) (r0 []CartItem, err error) {
	// Update metrics.
	start := time.Now()
	s.getCartMetrics.Count.Add(1)

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "cartservice.T.GetCart", trace.WithSpanKind(trace.SpanKindClient))
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
			s.getCartMetrics.ErrorCount.Add(1)
		}
		span.End()

		s.getCartMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
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
	s.getCartMetrics.BytesRequest.Put(float64(len(enc.Data())))
	var results []byte
	results, err = s.stub.Run(ctx, 2, enc.Data(), shardKey)
	if err != nil {
		return
	}
	s.getCartMetrics.BytesReply.Put(float64(len(results)))

	// Decode the results.
	dec := codegen.NewDecoder(results)
	r0 = serviceweaver_dec_slice_CartItem_b804d0c3(dec)
	err = dec.Error()
	return
}

func (s t_client_stub) EmptyCart(ctx context.Context, a0 string) (err error) {
	// Update metrics.
	start := time.Now()
	s.emptyCartMetrics.Count.Add(1)

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "cartservice.T.EmptyCart", trace.WithSpanKind(trace.SpanKindClient))
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
			s.emptyCartMetrics.ErrorCount.Add(1)
		}
		span.End()

		s.emptyCartMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
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
	s.emptyCartMetrics.BytesRequest.Put(float64(len(enc.Data())))
	var results []byte
	results, err = s.stub.Run(ctx, 1, enc.Data(), shardKey)
	if err != nil {
		return
	}
	s.emptyCartMetrics.BytesReply.Put(float64(len(results)))

	// Decode the results.
	dec := codegen.NewDecoder(results)
	err = dec.Error()
	return
}

type cartCache_client_stub struct {
	stub          codegen.Stub
	addMetrics    *codegen.MethodMetrics
	getMetrics    *codegen.MethodMetrics
	removeMetrics *codegen.MethodMetrics
}

func (s cartCache_client_stub) Add(ctx context.Context, a0 string, a1 []CartItem) (err error) {
	// Update metrics.
	start := time.Now()
	s.addMetrics.Count.Add(1)

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "cartservice.cartCache.Add", trace.WithSpanKind(trace.SpanKindClient))
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
			s.addMetrics.ErrorCount.Add(1)
		}
		span.End()

		s.addMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
	}()

	// Encode arguments.
	enc := codegen.NewEncoder()
	enc.String(a0)
	serviceweaver_enc_slice_CartItem_b804d0c3(enc, a1)

	// Set the shardKey.
	var r cartCacheRouter
	shardKey := _hashCartCache(r.Add(ctx, a0, a1))

	// Call the remote method.
	s.addMetrics.BytesRequest.Put(float64(len(enc.Data())))
	var results []byte
	results, err = s.stub.Run(ctx, 0, enc.Data(), shardKey)
	if err != nil {
		return
	}
	s.addMetrics.BytesReply.Put(float64(len(results)))

	// Decode the results.
	dec := codegen.NewDecoder(results)
	err = dec.Error()
	return
}

func (s cartCache_client_stub) Get(ctx context.Context, a0 string) (r0 []CartItem, err error) {
	// Update metrics.
	start := time.Now()
	s.getMetrics.Count.Add(1)

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "cartservice.cartCache.Get", trace.WithSpanKind(trace.SpanKindClient))
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
			s.getMetrics.ErrorCount.Add(1)
		}
		span.End()

		s.getMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
	}()

	// Preallocate a buffer of the right size.
	size := 0
	size += (4 + len(a0))
	enc := codegen.NewEncoder()
	enc.Reset(size)

	// Encode arguments.
	enc.String(a0)

	// Set the shardKey.
	var r cartCacheRouter
	shardKey := _hashCartCache(r.Get(ctx, a0))

	// Call the remote method.
	s.getMetrics.BytesRequest.Put(float64(len(enc.Data())))
	var results []byte
	results, err = s.stub.Run(ctx, 1, enc.Data(), shardKey)
	if err != nil {
		return
	}
	s.getMetrics.BytesReply.Put(float64(len(results)))

	// Decode the results.
	dec := codegen.NewDecoder(results)
	r0 = serviceweaver_dec_slice_CartItem_b804d0c3(dec)
	err = dec.Error()
	return
}

func (s cartCache_client_stub) Remove(ctx context.Context, a0 string) (r0 bool, err error) {
	// Update metrics.
	start := time.Now()
	s.removeMetrics.Count.Add(1)

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "cartservice.cartCache.Remove", trace.WithSpanKind(trace.SpanKindClient))
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
			s.removeMetrics.ErrorCount.Add(1)
		}
		span.End()

		s.removeMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
	}()

	// Preallocate a buffer of the right size.
	size := 0
	size += (4 + len(a0))
	enc := codegen.NewEncoder()
	enc.Reset(size)

	// Encode arguments.
	enc.String(a0)

	// Set the shardKey.
	var r cartCacheRouter
	shardKey := _hashCartCache(r.Remove(ctx, a0))

	// Call the remote method.
	s.removeMetrics.BytesRequest.Put(float64(len(enc.Data())))
	var results []byte
	results, err = s.stub.Run(ctx, 2, enc.Data(), shardKey)
	if err != nil {
		return
	}
	s.removeMetrics.BytesReply.Put(float64(len(results)))

	// Decode the results.
	dec := codegen.NewDecoder(results)
	r0 = dec.Bool()
	err = dec.Error()
	return
}

// Server stub implementations.

type t_server_stub struct {
	impl    T
	addLoad func(key uint64, load float64)
}

// GetStubFn implements the stub.Server interface.
func (s t_server_stub) GetStubFn(method string) func(ctx context.Context, args []byte) ([]byte, error) {
	switch method {
	case "AddItem":
		return s.addItem
	case "GetCart":
		return s.getCart
	case "EmptyCart":
		return s.emptyCart
	default:
		return nil
	}
}

func (s t_server_stub) addItem(ctx context.Context, args []byte) (res []byte, err error) {
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
	var a1 CartItem
	(&a1).WeaverUnmarshal(dec)

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	appErr := s.impl.AddItem(ctx, a0, a1)

	// Encode the results.
	enc := codegen.NewEncoder()
	enc.Error(appErr)
	return enc.Data(), nil
}

func (s t_server_stub) getCart(ctx context.Context, args []byte) (res []byte, err error) {
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
	r0, appErr := s.impl.GetCart(ctx, a0)

	// Encode the results.
	enc := codegen.NewEncoder()
	serviceweaver_enc_slice_CartItem_b804d0c3(enc, r0)
	enc.Error(appErr)
	return enc.Data(), nil
}

func (s t_server_stub) emptyCart(ctx context.Context, args []byte) (res []byte, err error) {
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
	appErr := s.impl.EmptyCart(ctx, a0)

	// Encode the results.
	enc := codegen.NewEncoder()
	enc.Error(appErr)
	return enc.Data(), nil
}

type cartCache_server_stub struct {
	impl    cartCache
	addLoad func(key uint64, load float64)
}

// GetStubFn implements the stub.Server interface.
func (s cartCache_server_stub) GetStubFn(method string) func(ctx context.Context, args []byte) ([]byte, error) {
	switch method {
	case "Add":
		return s.add
	case "Get":
		return s.get
	case "Remove":
		return s.remove
	default:
		return nil
	}
}

func (s cartCache_server_stub) add(ctx context.Context, args []byte) (res []byte, err error) {
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
	var a1 []CartItem
	a1 = serviceweaver_dec_slice_CartItem_b804d0c3(dec)
	var r cartCacheRouter
	s.addLoad(_hashCartCache(r.Add(ctx, a0, a1)), 1.0)

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	appErr := s.impl.Add(ctx, a0, a1)

	// Encode the results.
	enc := codegen.NewEncoder()
	enc.Error(appErr)
	return enc.Data(), nil
}

func (s cartCache_server_stub) get(ctx context.Context, args []byte) (res []byte, err error) {
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
	var r cartCacheRouter
	s.addLoad(_hashCartCache(r.Get(ctx, a0)), 1.0)

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	r0, appErr := s.impl.Get(ctx, a0)

	// Encode the results.
	enc := codegen.NewEncoder()
	serviceweaver_enc_slice_CartItem_b804d0c3(enc, r0)
	enc.Error(appErr)
	return enc.Data(), nil
}

func (s cartCache_server_stub) remove(ctx context.Context, args []byte) (res []byte, err error) {
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
	var r cartCacheRouter
	s.addLoad(_hashCartCache(r.Remove(ctx, a0)), 1.0)

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	r0, appErr := s.impl.Remove(ctx, a0)

	// Encode the results.
	enc := codegen.NewEncoder()
	enc.Bool(r0)
	enc.Error(appErr)
	return enc.Data(), nil
}

// AutoMarshal implementations.

var _ codegen.AutoMarshal = &CartItem{}

func (x *CartItem) WeaverMarshal(enc *codegen.Encoder) {
	if x == nil {
		panic(fmt.Errorf("CartItem.WeaverMarshal: nil receiver"))
	}
	enc.String(x.ProductID)
	enc.Int32(x.Quantity)
}

func (x *CartItem) WeaverUnmarshal(dec *codegen.Decoder) {
	if x == nil {
		panic(fmt.Errorf("CartItem.WeaverUnmarshal: nil receiver"))
	}
	x.ProductID = dec.String()
	x.Quantity = dec.Int32()
}

// Router methods.

// _hashCartCache returns a 64 bit hash of the provided value.
func _hashCartCache(r string) uint64 {
	var h codegen.Hasher
	h.WriteString(string(r))
	return h.Sum64()
}

// _orderedCodeCartCache returns an order-preserving serialization of the provided value.
func _orderedCodeCartCache(r string) codegen.OrderedCode {
	var enc codegen.OrderedEncoder
	enc.WriteString(string(r))
	return enc.Encode()
}

// Encoding/decoding implementations.

func serviceweaver_enc_slice_CartItem_b804d0c3(enc *codegen.Encoder, arg []CartItem) {
	if arg == nil {
		enc.Len(-1)
		return
	}
	enc.Len(len(arg))
	for i := 0; i < len(arg); i++ {
		(arg[i]).WeaverMarshal(enc)
	}
}

func serviceweaver_dec_slice_CartItem_b804d0c3(dec *codegen.Decoder) []CartItem {
	n := dec.Len()
	if n == -1 {
		return nil
	}
	res := make([]CartItem, n)
	for i := 0; i < n; i++ {
		(&res[i]).WeaverUnmarshal(dec)
	}
	return res
}

// Size implementations.

// serviceweaver_size_CartItem_f4da907b returns the size (in bytes) of the serialization
// of the provided type.
func serviceweaver_size_CartItem_f4da907b(x *CartItem) int {
	size := 0
	size += 0
	size += (4 + len(x.ProductID))
	size += 4
	return size
}
