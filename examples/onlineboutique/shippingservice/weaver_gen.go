// go:build !ignoreWeaverGen

package shippingservice

// Code generated by "weaver generate". DO NOT EDIT.
import (
	"context"
	"errors"
	"fmt"
	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/cartservice"
	"github.com/ServiceWeaver/weaver/examples/onlineboutique/types/money"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"reflect"
	"time"
)
var _ codegen.LatestVersion = codegen.Version[[0][10]struct{}]("You used 'weaver generate' codegen version 0.10.0, but you built your code with an incompatible weaver module version. Try upgrading 'weaver generate' and re-running it.")

func init() {
	codegen.Register(codegen.Registration{
		Name:        "github.com/ServiceWeaver/weaver/examples/onlineboutique/shippingservice/T",
		Iface:       reflect.TypeOf((*T)(nil)).Elem(),
		Impl:        reflect.TypeOf(impl{}),
		LocalStubFn: func(impl any, tracer trace.Tracer) any { return t_local_stub{impl: impl.(T), tracer: tracer} },
		ClientStubFn: func(stub codegen.Stub, caller string) any {
			return t_client_stub{stub: stub, getQuoteMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/onlineboutique/shippingservice/T", Method: "GetQuote"}), shipOrderMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/onlineboutique/shippingservice/T", Method: "ShipOrder"})}
		},
		ServerStubFn: func(impl any, addLoad func(uint64, float64)) codegen.Server {
			return t_server_stub{impl: impl.(T), addLoad: addLoad}
		},
		RefData: "",
	})
}

// weaver.Instance checks.
var _ weaver.InstanceOf[T] = (*impl)(nil)

// weaver.Router checks.
var _ weaver.Unrouted = (*impl)(nil)

// Local stub implementations.

type t_local_stub struct {
	impl   T
	tracer trace.Tracer
}

// Check that t_local_stub implements the T interface.
var _ T = (*t_local_stub)(nil)

func (s t_local_stub) GetQuote(ctx context.Context, a0 Address, a1 []cartservice.CartItem) (r0 money.T, err error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "shippingservice.T.GetQuote", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.GetQuote(ctx, a0, a1)
}

func (s t_local_stub) ShipOrder(ctx context.Context, a0 Address, a1 []cartservice.CartItem) (r0 string, err error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "shippingservice.T.ShipOrder", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.ShipOrder(ctx, a0, a1)
}

// Client stub implementations.

type t_client_stub struct {
	stub             codegen.Stub
	getQuoteMetrics  *codegen.MethodMetrics
	shipOrderMetrics *codegen.MethodMetrics
}

// Check that t_client_stub implements the T interface.
var _ T = (*t_client_stub)(nil)

func (s t_client_stub) GetQuote(ctx context.Context, a0 Address, a1 []cartservice.CartItem) (r0 money.T, err error) {
	// Update metrics.
	start := time.Now()
	s.getQuoteMetrics.Count.Add(1)

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "shippingservice.T.GetQuote", trace.WithSpanKind(trace.SpanKindClient))
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
			s.getQuoteMetrics.ErrorCount.Add(1)
		}
		span.End()

		s.getQuoteMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
	}()

	// Encode arguments.
	enc := codegen.NewEncoder()
	(a0).WeaverMarshal(enc)
	serviceweaver_enc_slice_CartItem_7a7ff11c(enc, a1)
	var shardKey uint64

	// Call the remote method.
	s.getQuoteMetrics.BytesRequest.Put(float64(len(enc.Data())))
	var results []byte
	results, err = s.stub.Run(ctx, 0, enc.Data(), shardKey)
	if err != nil {
		err = errors.Join(weaver.RemoteCallError, err)
		return
	}
	s.getQuoteMetrics.BytesReply.Put(float64(len(results)))

	// Decode the results.
	dec := codegen.NewDecoder(results)
	(&r0).WeaverUnmarshal(dec)
	err = dec.Error()
	return
}

func (s t_client_stub) ShipOrder(ctx context.Context, a0 Address, a1 []cartservice.CartItem) (r0 string, err error) {
	// Update metrics.
	start := time.Now()
	s.shipOrderMetrics.Count.Add(1)

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "shippingservice.T.ShipOrder", trace.WithSpanKind(trace.SpanKindClient))
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
			s.shipOrderMetrics.ErrorCount.Add(1)
		}
		span.End()

		s.shipOrderMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
	}()

	// Encode arguments.
	enc := codegen.NewEncoder()
	(a0).WeaverMarshal(enc)
	serviceweaver_enc_slice_CartItem_7a7ff11c(enc, a1)
	var shardKey uint64

	// Call the remote method.
	s.shipOrderMetrics.BytesRequest.Put(float64(len(enc.Data())))
	var results []byte
	results, err = s.stub.Run(ctx, 1, enc.Data(), shardKey)
	if err != nil {
		err = errors.Join(weaver.RemoteCallError, err)
		return
	}
	s.shipOrderMetrics.BytesReply.Put(float64(len(results)))

	// Decode the results.
	dec := codegen.NewDecoder(results)
	r0 = dec.String()
	err = dec.Error()
	return
}

// Server stub implementations.

type t_server_stub struct {
	impl    T
	addLoad func(key uint64, load float64)
}

// Check that t_server_stub implements the codegen.Server interface.
var _ codegen.Server = (*t_server_stub)(nil)

// GetStubFn implements the codegen.Server interface.
func (s t_server_stub) GetStubFn(method string) func(ctx context.Context, args []byte) ([]byte, error) {
	switch method {
	case "GetQuote":
		return s.getQuote
	case "ShipOrder":
		return s.shipOrder
	default:
		return nil
	}
}

func (s t_server_stub) getQuote(ctx context.Context, args []byte) (res []byte, err error) {
	// Catch and return any panics detected during encoding/decoding/rpc.
	defer func() {
		if err == nil {
			err = codegen.CatchPanics(recover())
		}
	}()

	// Decode arguments.
	dec := codegen.NewDecoder(args)
	var a0 Address
	(&a0).WeaverUnmarshal(dec)
	var a1 []cartservice.CartItem
	a1 = serviceweaver_dec_slice_CartItem_7a7ff11c(dec)

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	r0, appErr := s.impl.GetQuote(ctx, a0, a1)

	// Encode the results.
	enc := codegen.NewEncoder()
	(r0).WeaverMarshal(enc)
	enc.Error(appErr)
	return enc.Data(), nil
}

func (s t_server_stub) shipOrder(ctx context.Context, args []byte) (res []byte, err error) {
	// Catch and return any panics detected during encoding/decoding/rpc.
	defer func() {
		if err == nil {
			err = codegen.CatchPanics(recover())
		}
	}()

	// Decode arguments.
	dec := codegen.NewDecoder(args)
	var a0 Address
	(&a0).WeaverUnmarshal(dec)
	var a1 []cartservice.CartItem
	a1 = serviceweaver_dec_slice_CartItem_7a7ff11c(dec)

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	r0, appErr := s.impl.ShipOrder(ctx, a0, a1)

	// Encode the results.
	enc := codegen.NewEncoder()
	enc.String(r0)
	enc.Error(appErr)
	return enc.Data(), nil
}

// AutoMarshal implementations.

var _ codegen.AutoMarshal = (*Address)(nil)

type __is_Address[T ~struct {
	weaver.AutoMarshal
	StreetAddress string
	City          string
	State         string
	Country       string
	ZipCode       int32
}] struct{}

var _ __is_Address[Address]

func (x *Address) WeaverMarshal(enc *codegen.Encoder) {
	if x == nil {
		panic(fmt.Errorf("Address.WeaverMarshal: nil receiver"))
	}
	enc.String(x.StreetAddress)
	enc.String(x.City)
	enc.String(x.State)
	enc.String(x.Country)
	enc.Int32(x.ZipCode)
}

func (x *Address) WeaverUnmarshal(dec *codegen.Decoder) {
	if x == nil {
		panic(fmt.Errorf("Address.WeaverUnmarshal: nil receiver"))
	}
	x.StreetAddress = dec.String()
	x.City = dec.String()
	x.State = dec.String()
	x.Country = dec.String()
	x.ZipCode = dec.Int32()
}

// Encoding/decoding implementations.

func serviceweaver_enc_slice_CartItem_7a7ff11c(enc *codegen.Encoder, arg []cartservice.CartItem) {
	if arg == nil {
		enc.Len(-1)
		return
	}
	enc.Len(len(arg))
	for i := 0; i < len(arg); i++ {
		(arg[i]).WeaverMarshal(enc)
	}
}

func serviceweaver_dec_slice_CartItem_7a7ff11c(dec *codegen.Decoder) []cartservice.CartItem {
	n := dec.Len()
	if n == -1 {
		return nil
	}
	res := make([]cartservice.CartItem, n)
	for i := 0; i < n; i++ {
		(&res[i]).WeaverUnmarshal(dec)
	}
	return res
}
