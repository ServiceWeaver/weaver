// go:build !ignoreWeaverGen

package protos

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
		Name:  "github.com/ServiceWeaver/weaver/weavertest/internal/protos/PingPonger",
		Iface: reflect.TypeOf((*PingPonger)(nil)).Elem(),
		Impl:  reflect.TypeOf(impl{}),
		LocalStubFn: func(impl any, tracer trace.Tracer) any {
			return pingPonger_local_stub{impl: impl.(PingPonger), tracer: tracer}
		},
		ClientStubFn: func(stub codegen.Stub, caller string) any {
			return pingPonger_client_stub{stub: stub, pingMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/weavertest/internal/protos/PingPonger", Method: "Ping"})}
		},
		ServerStubFn: func(impl any, addLoad func(uint64, float64)) codegen.Server {
			return pingPonger_server_stub{impl: impl.(PingPonger), addLoad: addLoad}
		},
		RefData: "",
	})
}

// weaver.Instance checks.
var _ weaver.InstanceOf[PingPonger] = (*impl)(nil)

// weaver.Router checks.
var _ weaver.Unrouted = (*impl)(nil)

// Local stub implementations.

type pingPonger_local_stub struct {
	impl   PingPonger
	tracer trace.Tracer
}

// Check that pingPonger_local_stub implements the PingPonger interface.
var _ PingPonger = (*pingPonger_local_stub)(nil)

func (s pingPonger_local_stub) Ping(ctx context.Context, a0 *Ping) (r0 *Pong, err error) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "protos.PingPonger.Ping", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.Ping(ctx, a0)
}

// Client stub implementations.

type pingPonger_client_stub struct {
	stub        codegen.Stub
	pingMetrics *codegen.MethodMetrics
}

// Check that pingPonger_client_stub implements the PingPonger interface.
var _ PingPonger = (*pingPonger_client_stub)(nil)

func (s pingPonger_client_stub) Ping(ctx context.Context, a0 *Ping) (r0 *Pong, err error) {
	// Update metrics.
	start := time.Now()
	s.pingMetrics.Count.Add(1)

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "protos.PingPonger.Ping", trace.WithSpanKind(trace.SpanKindClient))
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
			s.pingMetrics.ErrorCount.Add(1)
		}
		span.End()

		s.pingMetrics.Latency.Put(float64(time.Since(start).Microseconds()))
	}()

	// Encode arguments.
	enc := codegen.NewEncoder()
	serviceweaver_enc_ptr_Ping_53efca65(enc, a0)
	var shardKey uint64

	// Call the remote method.
	s.pingMetrics.BytesRequest.Put(float64(len(enc.Data())))
	var results []byte
	results, err = s.stub.Run(ctx, 0, enc.Data(), shardKey)
	if err != nil {
		err = errors.Join(weaver.RemoteCallError, err)
		return
	}
	s.pingMetrics.BytesReply.Put(float64(len(results)))

	// Decode the results.
	dec := codegen.NewDecoder(results)
	r0 = serviceweaver_dec_ptr_Pong_10ae1a4e(dec)
	err = dec.Error()
	return
}

// Server stub implementations.

type pingPonger_server_stub struct {
	impl    PingPonger
	addLoad func(key uint64, load float64)
}

// Check that pingPonger_server_stub implements the codegen.Server interface.
var _ codegen.Server = (*pingPonger_server_stub)(nil)

// GetStubFn implements the codegen.Server interface.
func (s pingPonger_server_stub) GetStubFn(method string) func(ctx context.Context, args []byte) ([]byte, error) {
	switch method {
	case "Ping":
		return s.ping
	default:
		return nil
	}
}

func (s pingPonger_server_stub) ping(ctx context.Context, args []byte) (res []byte, err error) {
	// Catch and return any panics detected during encoding/decoding/rpc.
	defer func() {
		if err == nil {
			err = codegen.CatchPanics(recover())
		}
	}()

	// Decode arguments.
	dec := codegen.NewDecoder(args)
	var a0 *Ping
	a0 = serviceweaver_dec_ptr_Ping_53efca65(dec)

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	r0, appErr := s.impl.Ping(ctx, a0)

	// Encode the results.
	enc := codegen.NewEncoder()
	serviceweaver_enc_ptr_Pong_10ae1a4e(enc, r0)
	enc.Error(appErr)
	return enc.Data(), nil
}

// Encoding/decoding implementations.

func serviceweaver_enc_ptr_Ping_53efca65(enc *codegen.Encoder, arg *Ping) {
	if arg == nil {
		enc.Bool(false)
	} else {
		enc.Bool(true)
		enc.EncodeProto(arg)
	}
}

func serviceweaver_dec_ptr_Ping_53efca65(dec *codegen.Decoder) *Ping {
	if !dec.Bool() {
		return nil
	}
	var res Ping
	dec.DecodeProto(&res)
	return &res
}

func serviceweaver_enc_ptr_Pong_10ae1a4e(enc *codegen.Encoder, arg *Pong) {
	if arg == nil {
		enc.Bool(false)
	} else {
		enc.Bool(true)
		enc.EncodeProto(arg)
	}
}

func serviceweaver_dec_ptr_Pong_10ae1a4e(dec *codegen.Decoder) *Pong {
	if !dec.Bool() {
		return nil
	}
	var res Pong
	dec.DecodeProto(&res)
	return &res
}
