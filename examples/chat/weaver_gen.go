// Code generated by "weaver generate". DO NOT EDIT.
//go:build !ignoreWeaverGen

package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"reflect"
	"time"
)

var _ codegen.LatestVersion = codegen.Version[[0][17]struct{}](`

ERROR: You generated this file with 'weaver generate' v0.17.0 (codegen
version v0.17.0). The generated code is incompatible with the version of the
github.com/ServiceWeaver/weaver module that you're using. The weaver module
version can be found in your go.mod file or by running the following command.

    go list -m github.com/ServiceWeaver/weaver

We recommend updating the weaver module and the 'weaver generate' command by
running the following.

    go get github.com/ServiceWeaver/weaver@latest
    go install github.com/ServiceWeaver/weaver/cmd/weaver@latest

Then, re-run 'weaver generate' and re-build your code. If the problem persists,
please file an issue at https://github.com/ServiceWeaver/weaver/issues.

`)

func init() {
	codegen.Register(codegen.Registration{
		Name:  "github.com/ServiceWeaver/weaver/examples/chat/ImageScaler",
		Iface: reflect.TypeOf((*ImageScaler)(nil)).Elem(),
		Impl:  reflect.TypeOf(scaler{}),
		LocalStubFn: func(impl any, caller string, tracer trace.Tracer) any {
			return imageScaler_local_stub{impl: impl.(ImageScaler), tracer: tracer, scaleMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/chat/ImageScaler", Method: "Scale", Remote: false})}
		},
		ClientStubFn: func(stub codegen.Stub, caller string) any {
			return imageScaler_client_stub{stub: stub, scaleMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/chat/ImageScaler", Method: "Scale", Remote: true})}
		},
		ServerStubFn: func(impl any, addLoad func(uint64, float64)) codegen.Server {
			return imageScaler_server_stub{impl: impl.(ImageScaler), addLoad: addLoad}
		},
		RefData: "",
	})
	codegen.Register(codegen.Registration{
		Name:  "github.com/ServiceWeaver/weaver/examples/chat/LocalCache",
		Iface: reflect.TypeOf((*LocalCache)(nil)).Elem(),
		Impl:  reflect.TypeOf(localCache{}),
		LocalStubFn: func(impl any, caller string, tracer trace.Tracer) any {
			return localCache_local_stub{impl: impl.(LocalCache), tracer: tracer, getMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/chat/LocalCache", Method: "Get", Remote: false}), putMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/chat/LocalCache", Method: "Put", Remote: false})}
		},
		ClientStubFn: func(stub codegen.Stub, caller string) any {
			return localCache_client_stub{stub: stub, getMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/chat/LocalCache", Method: "Get", Remote: true}), putMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/chat/LocalCache", Method: "Put", Remote: true})}
		},
		ServerStubFn: func(impl any, addLoad func(uint64, float64)) codegen.Server {
			return localCache_server_stub{impl: impl.(LocalCache), addLoad: addLoad}
		},
		RefData: "",
	})
	codegen.Register(codegen.Registration{
		Name:      "github.com/ServiceWeaver/weaver/Main",
		Iface:     reflect.TypeOf((*weaver.Main)(nil)).Elem(),
		Impl:      reflect.TypeOf(server{}),
		Listeners: []string{"chat"},
		LocalStubFn: func(impl any, caller string, tracer trace.Tracer) any {
			return main_local_stub{impl: impl.(weaver.Main), tracer: tracer}
		},
		ClientStubFn: func(stub codegen.Stub, caller string) any { return main_client_stub{stub: stub} },
		ServerStubFn: func(impl any, addLoad func(uint64, float64)) codegen.Server {
			return main_server_stub{impl: impl.(weaver.Main), addLoad: addLoad}
		},
		RefData: "⟦7e1a0aa0:wEaVeReDgE:github.com/ServiceWeaver/weaver/Main→github.com/ServiceWeaver/weaver/examples/chat/SQLStore⟧\n⟦ae108c0d:wEaVeReDgE:github.com/ServiceWeaver/weaver/Main→github.com/ServiceWeaver/weaver/examples/chat/ImageScaler⟧\n⟦c86a1d44:wEaVeReDgE:github.com/ServiceWeaver/weaver/Main→github.com/ServiceWeaver/weaver/examples/chat/LocalCache⟧\n⟦7b9a3b0b:wEaVeRlIsTeNeRs:github.com/ServiceWeaver/weaver/Main→chat⟧\n",
	})
	codegen.Register(codegen.Registration{
		Name:  "github.com/ServiceWeaver/weaver/examples/chat/SQLStore",
		Iface: reflect.TypeOf((*SQLStore)(nil)).Elem(),
		Impl:  reflect.TypeOf(sqlStore{}),
		LocalStubFn: func(impl any, caller string, tracer trace.Tracer) any {
			return sQLStore_local_stub{impl: impl.(SQLStore), tracer: tracer, createPostMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/chat/SQLStore", Method: "CreatePost", Remote: false}), createThreadMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/chat/SQLStore", Method: "CreateThread", Remote: false}), getFeedMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/chat/SQLStore", Method: "GetFeed", Remote: false}), getImageMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/chat/SQLStore", Method: "GetImage", Remote: false})}
		},
		ClientStubFn: func(stub codegen.Stub, caller string) any {
			return sQLStore_client_stub{stub: stub, createPostMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/chat/SQLStore", Method: "CreatePost", Remote: true}), createThreadMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/chat/SQLStore", Method: "CreateThread", Remote: true}), getFeedMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/chat/SQLStore", Method: "GetFeed", Remote: true}), getImageMetrics: codegen.MethodMetricsFor(codegen.MethodLabels{Caller: caller, Component: "github.com/ServiceWeaver/weaver/examples/chat/SQLStore", Method: "GetImage", Remote: true})}
		},
		ServerStubFn: func(impl any, addLoad func(uint64, float64)) codegen.Server {
			return sQLStore_server_stub{impl: impl.(SQLStore), addLoad: addLoad}
		},
		RefData: "",
	})
}

// weaver.Instance checks.
var _ weaver.InstanceOf[ImageScaler] = (*scaler)(nil)
var _ weaver.InstanceOf[LocalCache] = (*localCache)(nil)
var _ weaver.InstanceOf[weaver.Main] = (*server)(nil)
var _ weaver.InstanceOf[SQLStore] = (*sqlStore)(nil)

// weaver.Router checks.
var _ weaver.Unrouted = (*scaler)(nil)
var _ weaver.Unrouted = (*localCache)(nil)
var _ weaver.Unrouted = (*server)(nil)
var _ weaver.Unrouted = (*sqlStore)(nil)

// Local stub implementations.

type imageScaler_local_stub struct {
	impl         ImageScaler
	tracer       trace.Tracer
	scaleMetrics *codegen.MethodMetrics
}

// Check that imageScaler_local_stub implements the ImageScaler interface.
var _ ImageScaler = (*imageScaler_local_stub)(nil)

func (s imageScaler_local_stub) Scale(ctx context.Context, a0 []byte, a1 int, a2 int) (r0 []byte, err error) {
	// Update metrics.
	begin := s.scaleMetrics.Begin()
	defer func() { s.scaleMetrics.End(begin, err != nil, 0, 0) }()
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "main.ImageScaler.Scale", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.Scale(ctx, a0, a1, a2)
}

type localCache_local_stub struct {
	impl       LocalCache
	tracer     trace.Tracer
	getMetrics *codegen.MethodMetrics
	putMetrics *codegen.MethodMetrics
}

// Check that localCache_local_stub implements the LocalCache interface.
var _ LocalCache = (*localCache_local_stub)(nil)

func (s localCache_local_stub) Get(ctx context.Context, a0 string) (r0 string, err error) {
	// Update metrics.
	begin := s.getMetrics.Begin()
	defer func() { s.getMetrics.End(begin, err != nil, 0, 0) }()
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "main.LocalCache.Get", trace.WithSpanKind(trace.SpanKindInternal))
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

func (s localCache_local_stub) Put(ctx context.Context, a0 string, a1 string) (err error) {
	// Update metrics.
	begin := s.putMetrics.Begin()
	defer func() { s.putMetrics.End(begin, err != nil, 0, 0) }()
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "main.LocalCache.Put", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.Put(ctx, a0, a1)
}

type main_local_stub struct {
	impl   weaver.Main
	tracer trace.Tracer
}

// Check that main_local_stub implements the weaver.Main interface.
var _ weaver.Main = (*main_local_stub)(nil)

type sQLStore_local_stub struct {
	impl                SQLStore
	tracer              trace.Tracer
	createPostMetrics   *codegen.MethodMetrics
	createThreadMetrics *codegen.MethodMetrics
	getFeedMetrics      *codegen.MethodMetrics
	getImageMetrics     *codegen.MethodMetrics
}

// Check that sQLStore_local_stub implements the SQLStore interface.
var _ SQLStore = (*sQLStore_local_stub)(nil)

func (s sQLStore_local_stub) CreatePost(ctx context.Context, a0 string, a1 time.Time, a2 ThreadID, a3 string) (err error) {
	// Update metrics.
	begin := s.createPostMetrics.Begin()
	defer func() { s.createPostMetrics.End(begin, err != nil, 0, 0) }()
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "main.SQLStore.CreatePost", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.CreatePost(ctx, a0, a1, a2, a3)
}

func (s sQLStore_local_stub) CreateThread(ctx context.Context, a0 string, a1 time.Time, a2 []string, a3 string, a4 []byte) (r0 ThreadID, err error) {
	// Update metrics.
	begin := s.createThreadMetrics.Begin()
	defer func() { s.createThreadMetrics.End(begin, err != nil, 0, 0) }()
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "main.SQLStore.CreateThread", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.CreateThread(ctx, a0, a1, a2, a3, a4)
}

func (s sQLStore_local_stub) GetFeed(ctx context.Context, a0 string) (r0 []Thread, err error) {
	// Update metrics.
	begin := s.getFeedMetrics.Begin()
	defer func() { s.getFeedMetrics.End(begin, err != nil, 0, 0) }()
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "main.SQLStore.GetFeed", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.GetFeed(ctx, a0)
}

func (s sQLStore_local_stub) GetImage(ctx context.Context, a0 string, a1 ImageID) (r0 []byte, err error) {
	// Update metrics.
	begin := s.getImageMetrics.Begin()
	defer func() { s.getImageMetrics.End(begin, err != nil, 0, 0) }()
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.tracer.Start(ctx, "main.SQLStore.GetImage", trace.WithSpanKind(trace.SpanKindInternal))
		defer func() {
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
		}()
	}

	return s.impl.GetImage(ctx, a0, a1)
}

// Client stub implementations.

type imageScaler_client_stub struct {
	stub         codegen.Stub
	scaleMetrics *codegen.MethodMetrics
}

// Check that imageScaler_client_stub implements the ImageScaler interface.
var _ ImageScaler = (*imageScaler_client_stub)(nil)

func (s imageScaler_client_stub) Scale(ctx context.Context, a0 []byte, a1 int, a2 int) (r0 []byte, err error) {
	// Update metrics.
	var requestBytes, replyBytes int
	begin := s.scaleMetrics.Begin()
	defer func() { s.scaleMetrics.End(begin, err != nil, requestBytes, replyBytes) }()

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "main.ImageScaler.Scale", trace.WithSpanKind(trace.SpanKindClient))
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
		}
		span.End()

	}()

	// Preallocate a buffer of the right size.
	size := 0
	size += (4 + (len(a0) * 1))
	size += 8
	size += 8
	enc := codegen.NewEncoder()
	enc.Reset(size)

	// Encode arguments.
	serviceweaver_enc_slice_byte_87461245(enc, a0)
	enc.Int(a1)
	enc.Int(a2)
	var shardKey uint64

	// Call the remote method.
	requestBytes = len(enc.Data())
	var results []byte
	results, err = s.stub.Run(ctx, 0, enc.Data(), shardKey)
	replyBytes = len(results)
	if err != nil {
		err = errors.Join(weaver.RemoteCallError, err)
		return
	}

	// Decode the results.
	dec := codegen.NewDecoder(results)
	r0 = serviceweaver_dec_slice_byte_87461245(dec)
	err = dec.Error()
	return
}

type localCache_client_stub struct {
	stub       codegen.Stub
	getMetrics *codegen.MethodMetrics
	putMetrics *codegen.MethodMetrics
}

// Check that localCache_client_stub implements the LocalCache interface.
var _ LocalCache = (*localCache_client_stub)(nil)

func (s localCache_client_stub) Get(ctx context.Context, a0 string) (r0 string, err error) {
	// Update metrics.
	var requestBytes, replyBytes int
	begin := s.getMetrics.Begin()
	defer func() { s.getMetrics.End(begin, err != nil, requestBytes, replyBytes) }()

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "main.LocalCache.Get", trace.WithSpanKind(trace.SpanKindClient))
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
		}
		span.End()

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
	requestBytes = len(enc.Data())
	var results []byte
	results, err = s.stub.Run(ctx, 0, enc.Data(), shardKey)
	replyBytes = len(results)
	if err != nil {
		err = errors.Join(weaver.RemoteCallError, err)
		return
	}

	// Decode the results.
	dec := codegen.NewDecoder(results)
	r0 = dec.String()
	err = dec.Error()
	return
}

func (s localCache_client_stub) Put(ctx context.Context, a0 string, a1 string) (err error) {
	// Update metrics.
	var requestBytes, replyBytes int
	begin := s.putMetrics.Begin()
	defer func() { s.putMetrics.End(begin, err != nil, requestBytes, replyBytes) }()

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "main.LocalCache.Put", trace.WithSpanKind(trace.SpanKindClient))
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
		}
		span.End()

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
	requestBytes = len(enc.Data())
	var results []byte
	results, err = s.stub.Run(ctx, 1, enc.Data(), shardKey)
	replyBytes = len(results)
	if err != nil {
		err = errors.Join(weaver.RemoteCallError, err)
		return
	}

	// Decode the results.
	dec := codegen.NewDecoder(results)
	err = dec.Error()
	return
}

type main_client_stub struct {
	stub codegen.Stub
}

// Check that main_client_stub implements the weaver.Main interface.
var _ weaver.Main = (*main_client_stub)(nil)

type sQLStore_client_stub struct {
	stub                codegen.Stub
	createPostMetrics   *codegen.MethodMetrics
	createThreadMetrics *codegen.MethodMetrics
	getFeedMetrics      *codegen.MethodMetrics
	getImageMetrics     *codegen.MethodMetrics
}

// Check that sQLStore_client_stub implements the SQLStore interface.
var _ SQLStore = (*sQLStore_client_stub)(nil)

func (s sQLStore_client_stub) CreatePost(ctx context.Context, a0 string, a1 time.Time, a2 ThreadID, a3 string) (err error) {
	// Update metrics.
	var requestBytes, replyBytes int
	begin := s.createPostMetrics.Begin()
	defer func() { s.createPostMetrics.End(begin, err != nil, requestBytes, replyBytes) }()

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "main.SQLStore.CreatePost", trace.WithSpanKind(trace.SpanKindClient))
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
		}
		span.End()

	}()

	// Encode arguments.
	enc := codegen.NewEncoder()
	enc.String(a0)
	enc.EncodeBinaryMarshaler(&a1)
	enc.Int64((int64)(a2))
	enc.String(a3)
	var shardKey uint64

	// Call the remote method.
	requestBytes = len(enc.Data())
	var results []byte
	results, err = s.stub.Run(ctx, 0, enc.Data(), shardKey)
	replyBytes = len(results)
	if err != nil {
		err = errors.Join(weaver.RemoteCallError, err)
		return
	}

	// Decode the results.
	dec := codegen.NewDecoder(results)
	err = dec.Error()
	return
}

func (s sQLStore_client_stub) CreateThread(ctx context.Context, a0 string, a1 time.Time, a2 []string, a3 string, a4 []byte) (r0 ThreadID, err error) {
	// Update metrics.
	var requestBytes, replyBytes int
	begin := s.createThreadMetrics.Begin()
	defer func() { s.createThreadMetrics.End(begin, err != nil, requestBytes, replyBytes) }()

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "main.SQLStore.CreateThread", trace.WithSpanKind(trace.SpanKindClient))
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
		}
		span.End()

	}()

	// Encode arguments.
	enc := codegen.NewEncoder()
	enc.String(a0)
	enc.EncodeBinaryMarshaler(&a1)
	serviceweaver_enc_slice_string_4af10117(enc, a2)
	enc.String(a3)
	serviceweaver_enc_slice_byte_87461245(enc, a4)
	var shardKey uint64

	// Call the remote method.
	requestBytes = len(enc.Data())
	var results []byte
	results, err = s.stub.Run(ctx, 1, enc.Data(), shardKey)
	replyBytes = len(results)
	if err != nil {
		err = errors.Join(weaver.RemoteCallError, err)
		return
	}

	// Decode the results.
	dec := codegen.NewDecoder(results)
	*(*int64)(&r0) = dec.Int64()
	err = dec.Error()
	return
}

func (s sQLStore_client_stub) GetFeed(ctx context.Context, a0 string) (r0 []Thread, err error) {
	// Update metrics.
	var requestBytes, replyBytes int
	begin := s.getFeedMetrics.Begin()
	defer func() { s.getFeedMetrics.End(begin, err != nil, requestBytes, replyBytes) }()

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "main.SQLStore.GetFeed", trace.WithSpanKind(trace.SpanKindClient))
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
		}
		span.End()

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
	requestBytes = len(enc.Data())
	var results []byte
	results, err = s.stub.Run(ctx, 2, enc.Data(), shardKey)
	replyBytes = len(results)
	if err != nil {
		err = errors.Join(weaver.RemoteCallError, err)
		return
	}

	// Decode the results.
	dec := codegen.NewDecoder(results)
	r0 = serviceweaver_dec_slice_Thread_511e1469(dec)
	err = dec.Error()
	return
}

func (s sQLStore_client_stub) GetImage(ctx context.Context, a0 string, a1 ImageID) (r0 []byte, err error) {
	// Update metrics.
	var requestBytes, replyBytes int
	begin := s.getImageMetrics.Begin()
	defer func() { s.getImageMetrics.End(begin, err != nil, requestBytes, replyBytes) }()

	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		// Create a child span for this method.
		ctx, span = s.stub.Tracer().Start(ctx, "main.SQLStore.GetImage", trace.WithSpanKind(trace.SpanKindClient))
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
		}
		span.End()

	}()

	// Preallocate a buffer of the right size.
	size := 0
	size += (4 + len(a0))
	size += 8
	enc := codegen.NewEncoder()
	enc.Reset(size)

	// Encode arguments.
	enc.String(a0)
	enc.Int64((int64)(a1))
	var shardKey uint64

	// Call the remote method.
	requestBytes = len(enc.Data())
	var results []byte
	results, err = s.stub.Run(ctx, 3, enc.Data(), shardKey)
	replyBytes = len(results)
	if err != nil {
		err = errors.Join(weaver.RemoteCallError, err)
		return
	}

	// Decode the results.
	dec := codegen.NewDecoder(results)
	r0 = serviceweaver_dec_slice_byte_87461245(dec)
	err = dec.Error()
	return
}

// Server stub implementations.

type imageScaler_server_stub struct {
	impl    ImageScaler
	addLoad func(key uint64, load float64)
}

// Check that imageScaler_server_stub implements the codegen.Server interface.
var _ codegen.Server = (*imageScaler_server_stub)(nil)

// GetStubFn implements the codegen.Server interface.
func (s imageScaler_server_stub) GetStubFn(method string) func(ctx context.Context, args []byte) ([]byte, error) {
	switch method {
	case "Scale":
		return s.scale
	default:
		return nil
	}
}

func (s imageScaler_server_stub) scale(ctx context.Context, args []byte) (res []byte, err error) {
	// Catch and return any panics detected during encoding/decoding/rpc.
	defer func() {
		if err == nil {
			err = codegen.CatchPanics(recover())
		}
	}()

	// Decode arguments.
	dec := codegen.NewDecoder(args)
	var a0 []byte
	a0 = serviceweaver_dec_slice_byte_87461245(dec)
	var a1 int
	a1 = dec.Int()
	var a2 int
	a2 = dec.Int()

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	r0, appErr := s.impl.Scale(ctx, a0, a1, a2)

	// Encode the results.
	enc := codegen.NewEncoder()
	serviceweaver_enc_slice_byte_87461245(enc, r0)
	enc.Error(appErr)
	return enc.Data(), nil
}

type localCache_server_stub struct {
	impl    LocalCache
	addLoad func(key uint64, load float64)
}

// Check that localCache_server_stub implements the codegen.Server interface.
var _ codegen.Server = (*localCache_server_stub)(nil)

// GetStubFn implements the codegen.Server interface.
func (s localCache_server_stub) GetStubFn(method string) func(ctx context.Context, args []byte) ([]byte, error) {
	switch method {
	case "Get":
		return s.get
	case "Put":
		return s.put
	default:
		return nil
	}
}

func (s localCache_server_stub) get(ctx context.Context, args []byte) (res []byte, err error) {
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
	r0, appErr := s.impl.Get(ctx, a0)

	// Encode the results.
	enc := codegen.NewEncoder()
	enc.String(r0)
	enc.Error(appErr)
	return enc.Data(), nil
}

func (s localCache_server_stub) put(ctx context.Context, args []byte) (res []byte, err error) {
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
	appErr := s.impl.Put(ctx, a0, a1)

	// Encode the results.
	enc := codegen.NewEncoder()
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
	default:
		return nil
	}
}

type sQLStore_server_stub struct {
	impl    SQLStore
	addLoad func(key uint64, load float64)
}

// Check that sQLStore_server_stub implements the codegen.Server interface.
var _ codegen.Server = (*sQLStore_server_stub)(nil)

// GetStubFn implements the codegen.Server interface.
func (s sQLStore_server_stub) GetStubFn(method string) func(ctx context.Context, args []byte) ([]byte, error) {
	switch method {
	case "CreatePost":
		return s.createPost
	case "CreateThread":
		return s.createThread
	case "GetFeed":
		return s.getFeed
	case "GetImage":
		return s.getImage
	default:
		return nil
	}
}

func (s sQLStore_server_stub) createPost(ctx context.Context, args []byte) (res []byte, err error) {
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
	var a1 time.Time
	dec.DecodeBinaryUnmarshaler(&a1)
	var a2 ThreadID
	*(*int64)(&a2) = dec.Int64()
	var a3 string
	a3 = dec.String()

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	appErr := s.impl.CreatePost(ctx, a0, a1, a2, a3)

	// Encode the results.
	enc := codegen.NewEncoder()
	enc.Error(appErr)
	return enc.Data(), nil
}

func (s sQLStore_server_stub) createThread(ctx context.Context, args []byte) (res []byte, err error) {
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
	var a1 time.Time
	dec.DecodeBinaryUnmarshaler(&a1)
	var a2 []string
	a2 = serviceweaver_dec_slice_string_4af10117(dec)
	var a3 string
	a3 = dec.String()
	var a4 []byte
	a4 = serviceweaver_dec_slice_byte_87461245(dec)

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	r0, appErr := s.impl.CreateThread(ctx, a0, a1, a2, a3, a4)

	// Encode the results.
	enc := codegen.NewEncoder()
	enc.Int64((int64)(r0))
	enc.Error(appErr)
	return enc.Data(), nil
}

func (s sQLStore_server_stub) getFeed(ctx context.Context, args []byte) (res []byte, err error) {
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
	r0, appErr := s.impl.GetFeed(ctx, a0)

	// Encode the results.
	enc := codegen.NewEncoder()
	serviceweaver_enc_slice_Thread_511e1469(enc, r0)
	enc.Error(appErr)
	return enc.Data(), nil
}

func (s sQLStore_server_stub) getImage(ctx context.Context, args []byte) (res []byte, err error) {
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
	var a1 ImageID
	*(*int64)(&a1) = dec.Int64()

	// TODO(rgrandl): The deferred function above will recover from panics in the
	// user code: fix this.
	// Call the local method.
	r0, appErr := s.impl.GetImage(ctx, a0, a1)

	// Encode the results.
	enc := codegen.NewEncoder()
	serviceweaver_enc_slice_byte_87461245(enc, r0)
	enc.Error(appErr)
	return enc.Data(), nil
}

// AutoMarshal implementations.

var _ codegen.AutoMarshal = (*Post)(nil)

type __is_Post[T ~struct {
	weaver.AutoMarshal
	ID      PostID
	Creator string
	When    time.Time
	Text    string
	ImageID ImageID
}] struct{}

var _ __is_Post[Post]

func (x *Post) WeaverMarshal(enc *codegen.Encoder) {
	if x == nil {
		panic(fmt.Errorf("Post.WeaverMarshal: nil receiver"))
	}
	enc.Int64((int64)(x.ID))
	enc.String(x.Creator)
	enc.EncodeBinaryMarshaler(&x.When)
	enc.String(x.Text)
	enc.Int64((int64)(x.ImageID))
}

func (x *Post) WeaverUnmarshal(dec *codegen.Decoder) {
	if x == nil {
		panic(fmt.Errorf("Post.WeaverUnmarshal: nil receiver"))
	}
	*(*int64)(&x.ID) = dec.Int64()
	x.Creator = dec.String()
	dec.DecodeBinaryUnmarshaler(&x.When)
	x.Text = dec.String()
	*(*int64)(&x.ImageID) = dec.Int64()
}

var _ codegen.AutoMarshal = (*Thread)(nil)

type __is_Thread[T ~struct {
	weaver.AutoMarshal
	ID    ThreadID
	Posts []Post
}] struct{}

var _ __is_Thread[Thread]

func (x *Thread) WeaverMarshal(enc *codegen.Encoder) {
	if x == nil {
		panic(fmt.Errorf("Thread.WeaverMarshal: nil receiver"))
	}
	enc.Int64((int64)(x.ID))
	serviceweaver_enc_slice_Post_29a9ee83(enc, x.Posts)
}

func (x *Thread) WeaverUnmarshal(dec *codegen.Decoder) {
	if x == nil {
		panic(fmt.Errorf("Thread.WeaverUnmarshal: nil receiver"))
	}
	*(*int64)(&x.ID) = dec.Int64()
	x.Posts = serviceweaver_dec_slice_Post_29a9ee83(dec)
}

func serviceweaver_enc_slice_Post_29a9ee83(enc *codegen.Encoder, arg []Post) {
	if arg == nil {
		enc.Len(-1)
		return
	}
	enc.Len(len(arg))
	for i := 0; i < len(arg); i++ {
		(arg[i]).WeaverMarshal(enc)
	}
}

func serviceweaver_dec_slice_Post_29a9ee83(dec *codegen.Decoder) []Post {
	n := dec.Len()
	if n == -1 {
		return nil
	}
	res := make([]Post, n)
	for i := 0; i < n; i++ {
		(&res[i]).WeaverUnmarshal(dec)
	}
	return res
}

// Encoding/decoding implementations.

func serviceweaver_enc_slice_byte_87461245(enc *codegen.Encoder, arg []byte) {
	if arg == nil {
		enc.Len(-1)
		return
	}
	enc.Len(len(arg))
	for i := 0; i < len(arg); i++ {
		enc.Byte(arg[i])
	}
}

func serviceweaver_dec_slice_byte_87461245(dec *codegen.Decoder) []byte {
	n := dec.Len()
	if n == -1 {
		return nil
	}
	res := make([]byte, n)
	for i := 0; i < n; i++ {
		res[i] = dec.Byte()
	}
	return res
}

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

func serviceweaver_enc_slice_Thread_511e1469(enc *codegen.Encoder, arg []Thread) {
	if arg == nil {
		enc.Len(-1)
		return
	}
	enc.Len(len(arg))
	for i := 0; i < len(arg); i++ {
		(arg[i]).WeaverMarshal(enc)
	}
}

func serviceweaver_dec_slice_Thread_511e1469(dec *codegen.Decoder) []Thread {
	n := dec.Len()
	if n == -1 {
		return nil
	}
	res := make([]Thread, n)
	for i := 0; i < n; i++ {
		(&res[i]).WeaverUnmarshal(dec)
	}
	return res
}
