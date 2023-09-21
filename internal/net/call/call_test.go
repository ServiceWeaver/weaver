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

package call_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/internal/cond"
	"github.com/ServiceWeaver/weaver/internal/net/call"
	"github.com/ServiceWeaver/weaver/internal/traceio"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type resolverMaker func(...call.Endpoint) call.Resolver

var (
	echoKey       = call.MakeMethodKey("", "echo")
	whoKey        = call.MakeMethodKey("", "who")
	errorKey      = call.MakeMethodKey("", "error")
	cancelWaitKey = call.MakeMethodKey("", "cancelwait")
	sleepKey      = call.MakeMethodKey("", "sleep")
	customKey     = call.MakeMethodKey("", "custom")
	handlers      = makeHandlerMap()
	tlsConfig     = makeTLSConfig()

	resolverMakers = map[string]resolverMaker{
		"Constant": func(addrs ...call.Endpoint) call.Resolver {
			return call.NewConstantResolver(addrs...)
		},
		"NonConstant": func(addrs ...call.Endpoint) call.Resolver {
			return newDynamicResolver(addrs...)
		},
	}
)

func makeHandlerMap() *call.HandlerMap {
	m := call.NewHandlerMap()
	m.Set("", "echo", echoHandler)
	m.Set("", "error", errorHandler)
	m.Set("", "cancelwait", cancelWaitHandler)
	m.Set("", "sleep", sleepHandler)
	m.Set("", "custom", customHandler)
	return m
}

func makeTLSConfig() *tls.Config {
	certPem := []byte(`-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIQIRi6zePL6mKjOipn+dNuaTAKBggqhkjOPQQDAjASMRAw
DgYDVQQKEwdBY21lIENvMB4XDTE3MTAyMDE5NDMwNloXDTE4MTAyMDE5NDMwNlow
EjEQMA4GA1UEChMHQWNtZSBDbzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABD0d
7VNhbWvZLWPuj/RtHFjvtJBEwOkhbN/BnnE8rnZR8+sbwnc/KhCk3FhnpHZnQz7B
5aETbbIgmuvewdjvSBSjYzBhMA4GA1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggr
BgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MCkGA1UdEQQiMCCCDmxvY2FsaG9zdDo1
NDUzgg4xMjcuMC4wLjE6NTQ1MzAKBggqhkjOPQQDAgNIADBFAiEA2zpJEPQyz6/l
Wf86aX6PepsntZv2GYlA5UpabfT2EZICICpJ5h/iI+i341gBmLiAFQOyTDT+/wQc
6MF9+Yw1Yy0t
-----END CERTIFICATE-----`)
	keyPem := []byte(`-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIIrYSSNQFaA2Hwf1duRSxKtLYX5CB04fSeQ6tF1aY/PuoAoGCCqGSM49
AwEHoUQDQgAEPR3tU2Fta9ktY+6P9G0cWO+0kETA6SFs38GecTyudlHz6xvCdz8q
EKTcWGekdmdDPsHloRNtsiCa697B2O9IFA==
-----END EC PRIVATE KEY-----`)
	cert, err := tls.X509KeyPair(certPem, keyPem)
	if err != nil {
		log.Fatal(err)
	}
	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		ClientAuth:         tls.RequireAnyClientCert,
		InsecureSkipVerify: true, // ok when VerifyPeerCertificate present
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) != 1 {
				return fmt.Errorf("expected single cert, got %d", len(rawCerts))
			}
			if !bytes.Equal(cert.Certificate[0], rawCerts[0]) {
				return fmt.Errorf("invalid peer certificate")
			}
			return nil
		},
	}
}

type testListener struct {
	net.Listener
	tlsConfig *tls.Config
}

var _ call.Listener = &testListener{}

func (l testListener) Accept() (net.Conn, *call.HandlerMap, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, nil, err
	}
	if l.tlsConfig != nil {
		tlsConn := tls.Server(conn, l.tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			return nil, nil, fmt.Errorf("TLS handshake error: %w", err)
		}
		conn = tlsConn
	}
	return conn, handlers, err
}

// startServers starts a new long-running server for each tested network
// protocol (e.g., "tcp"), returning the endpoints for those servers.
func startServers(ctx context.Context, opts call.ServerOptions) map[string]call.Endpoint {
	serve := func(lis call.Listener) {
		call.Serve(ctx, lis, opts)
	}

	// Start the server that uses the TCP protocol.
	tcpListener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	go serve(testListener{Listener: tcpListener})

	// Start the server that uses the MTLS protocol over TCP.
	mtlsListener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	go serve(testListener{Listener: mtlsListener, tlsConfig: tlsConfig})

	return map[string]call.Endpoint{
		"tcp":  call.TCP(tcpListener.Addr().String()),
		"mtls": call.MTLS(tlsConfig, call.TCP(mtlsListener.Addr().String())),
	}
}

const (
	// testTimeout is used to time out broken tests without waiting for
	// an unbounded amount of time.
	testTimeout = time.Second * 10

	// shortDelay is used in some tests to delay some action. It should be
	// much smaller than testTimeout.
	shortDelay = time.Millisecond * 100

	// delaySlop is extra delay added to account for usual jitter in execution
	// times (e.g., scheduling delays). It should be much smaller than testTimeout.
	delaySlop = time.Second
)

// handlersFor returns a copy of handlers with a whoHandler that returns
// `server`.
func handlersFor(server string) *call.HandlerMap {
	h := makeHandlerMap()
	h.Set("", "who", whoHandler(server))
	return h
}

func echoHandler(_ context.Context, arg []byte) ([]byte, error) {
	return arg, nil
}

func whoHandler(name string) call.Handler {
	return func(context.Context, []byte) ([]byte, error) {
		return []byte(name), nil
	}
}

// Registered function that should be used on server-side of some RPCs.
var (
	registeredFuncsMu     sync.Mutex
	registeredFuncs       = map[int]func(context.Context) ([]byte, error){}
	nextRegisteredFuncKey int
)

// runAtServer makes an RPC to client and arranges to run fn() on the
// server-side as the handler for that RPC.
func runAtServer(ctx context.Context, client call.Connection, opts call.CallOptions, fn func(context.Context) ([]byte, error)) ([]byte, error) {
	// Register fn in registeredFuncs and send its key as the RPC argument.
	registeredFuncsMu.Lock()
	key := nextRegisteredFuncKey
	nextRegisteredFuncKey++
	registeredFuncs[key] = fn
	registeredFuncsMu.Unlock()

	defer func() {
		registeredFuncsMu.Lock()
		delete(registeredFuncs, key)
		registeredFuncsMu.Unlock()
	}()

	return client.Call(ctx, customKey, []byte(fmt.Sprint(key)), opts)
}

// customHandler reads an integer key from arg and runs registeredFuncs[key].
func customHandler(ctx context.Context, arg []byte) ([]byte, error) {
	key, err := strconv.Atoi(string(arg))
	if err != nil {
		return nil, fmt.Errorf("customHandler: bad argument %v (must be a number)", string(arg))
	}

	registeredFuncsMu.Lock()
	fn, ok := registeredFuncs[key]
	registeredFuncsMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("customHandler: function %d not found", key)
	}

	return fn(ctx)
}

func errorHandler(_ context.Context, arg []byte) ([]byte, error) {
	return nil, fmt.Errorf("%w: %s", os.ErrInvalid, string(arg))
}

var cancelCount int64

func cancelWaitHandler(ctx context.Context, _ []byte) ([]byte, error) {
	t := time.NewTimer(testTimeout)
	select {
	case <-t.C:
		return nil, fmt.Errorf("cancelWait handler timed out")
	case <-ctx.Done():
		atomic.AddInt64(&cancelCount, 1)
		t.Stop()
		return nil, ctx.Err()
	}
}

// sleepHandler sleeps for the provided amount of time. arg must be parseable
// by time.ParseDuration.
func sleepHandler(ctx context.Context, arg []byte) ([]byte, error) {
	duration, err := time.ParseDuration(string(arg))
	if err != nil {
		return nil, err
	}

	sleep := time.NewTimer(duration)
	timeout := time.NewTimer(testTimeout)
	defer sleep.Stop()
	defer timeout.Stop()
	select {
	case <-sleep.C:
		return nil, nil
	case <-timeout.C:
		return nil, fmt.Errorf("sleepHandler timed out")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// getClientConn returns a fresh RPC client to a single long-running server
// that uses the given network protocol, using maker to make the resolver.
func getClientConn(t testing.TB, protocol string, endpoint call.Endpoint, maker resolverMaker) call.Connection {
	t.Helper()
	ctx := context.Background()

	opts := call.ClientOptions{Logger: logger(t)}
	client, err := call.Connect(ctx, maker(endpoint), opts)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

// pipe is a thin-wrapper around net.Pipe that schedules for the pipes to be
// closed when the unit test t ends.
func pipe(t testing.TB) (client net.Conn, server net.Conn) {
	c, s := net.Pipe()
	t.Cleanup(func() {
		c.Close()
		s.Close()
	})
	return c, s
}

// server returns a fake pipe-based endpoint with the given name. The returned
// server can be dialed repeatedly. Every dial returns a fresh network
// connection connected to a server running handlersFor(name).
func server(t testing.TB, name string) call.Endpoint {
	return &pipeEndpoint{
		name:     name,
		handlers: handlersFor(name),
		t:        t,
	}
}

// servers returns fake servers named 0, ..., n-1.
func servers(t testing.TB, n int) []call.Endpoint {
	endpoints := make([]call.Endpoint, n)
	for i := 0; i < n; i++ {
		endpoints[i] = server(t, strconv.Itoa(i))
	}
	return endpoints
}

// pipeEndpoint is a pipe-based endpoint. Every time Dial is called, Dial
// creates a client and server pipe by calling net.Pipe, runs an RPC server on
// the server pipe, and returns the client pipe.
type pipeEndpoint struct {
	name     string
	handlers *call.HandlerMap
	t        testing.TB
}

func (p *pipeEndpoint) Dial(context.Context) (net.Conn, error) {
	client, server := pipe(p.t)
	// Note: do not use passed in context since we want the server to
	// be independent of the context in which the client is running.
	opts := call.ServerOptions{Logger: logger(p.t)}
	call.ServeOn(context.Background(), server, p.handlers, opts)
	return client, nil
}

func (p *pipeEndpoint) Address() string {
	return fmt.Sprintf("pipe://%s", p.name)
}

// connEndpoint is an endpoint that always returns the provided net.Conn.
type connEndpoint struct {
	name string
	conn net.Conn
}

func (c *connEndpoint) Dial(context.Context) (net.Conn, error) {
	return c.conn, nil
}

func (c *connEndpoint) Address() string {
	return fmt.Sprintf("conn://%s", c.name)
}

// connEndpoint is an endpoint that returns the provided net.Conns in order,
// one per call to Dial. Once the net.Conns have been exhausted, Dial returns
// an error.
type connsEndpoint struct {
	name string

	mu    sync.Mutex
	conns []net.Conn
}

func (c *connsEndpoint) Dial(context.Context) (net.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.conns) == 0 {
		return nil, fmt.Errorf("conns used up")
	}
	conn := c.conns[0]
	c.conns = c.conns[1:]
	return conn, nil
}

func (c *connsEndpoint) Address() string {
	return fmt.Sprintf("conn://%s", c.name)
}

// deadEndpoint is an endpoint that emulates a dead server by having Dial always return nil.
type deadEndpoint struct {
	name string
}

func (d *deadEndpoint) Dial(context.Context) (net.Conn, error) {
	return nil, fmt.Errorf("dead backend %s", d.name)
}

func (d *deadEndpoint) Address() string {
	return fmt.Sprintf("dead://%s", d.name)
}

// hangingConn is a net.Conn that never actually responds to calls.
type hangingConn struct {
	net.Conn // Inherit normal behavior from wrapped conn
}

func (h hangingConn) Write(b []byte) (n int, err error) {
	// Throw away the message.
	fmt.Fprintf(os.Stderr, "dropping write to %s\n", h.RemoteAddr().String())
	return len(b), nil
}

// hangingEndpoint returns a hangingConn.
type hangingEndpoint struct {
	call.Endpoint
}

var _ call.Endpoint = hangingEndpoint{}

func (h hangingEndpoint) Dial(ctx context.Context) (net.Conn, error) {
	// Make real connection and wrap it inside a hangingConn.
	c, err := h.Endpoint.Dial(ctx)
	if err != nil {
		return nil, err
	}
	return hangingConn{c}, nil
}

// callTester manages operation and shutdown of a call test.
type callTester struct {
	t      testing.TB
	ctx    context.Context
	cancel func()
	done   []chan struct{} // Channels are closed as sub-goroutines finish
}

func startTest(t testing.TB) *callTester {
	ct := &callTester{t: t}
	t.Helper()
	ct.ctx, ct.cancel = context.WithTimeout(context.Background(), testTimeout)

	// When exiting, wait for goroutines to finish.
	t.Cleanup(func() {
		ct.cancel()
		for _, c := range ct.done {
			<-c
		}
	})
	return ct
}

// fork runs fn in a goroutine and waits for it to exit when the test ends.
func (ct *callTester) fork(fn func()) {
	done := make(chan struct{})
	ct.done = append(ct.done, done)
	go func() {
		defer close(done)
		fn()
	}()
}

// startTCPServer starts a TCP server and returns its endpoint.
func (ct *callTester) startTCPServer() call.Endpoint {
	t := ct.t
	t.Helper()
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("server listen failed: %v", err)
	}
	t.Logf("server %q", lis.Addr().String())
	ct.fork(func() {
		opts := call.ServerOptions{Logger: logger(t)}
		err := call.Serve(ct.ctx, testListener{Listener: lis}, opts)
		if err != ct.ctx.Err() {
			t.Errorf("unexpected error from Serve: %v", err)
		}
	})
	return call.TCP(lis.Addr().String())
}

// connect creates a client connected via the specified resolver.
func (ct *callTester) connect(resolver call.Resolver) call.Connection {
	t := ct.t
	t.Helper()
	client, err := call.Connect(ct.ctx, resolver, call.ClientOptions{Logger: logger(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		client.Close()
	})
	return client
}

// waitUntil repeatedly calls f until it returns true, with a small delay
// between invocations. If f doesn't return true before the testTimeout is
// reached, the test is failed.
func waitUntil(t testing.TB, f func() bool) {
	t.Helper()
	timeout := time.NewTimer(testTimeout)
	ticker := time.NewTicker(shortDelay)
	defer timeout.Stop()
	defer ticker.Stop()
	for {
		select {
		case <-timeout.C:
			t.Fatal("test timeout")
		case <-ticker.C:
			if f() {
				return
			}
		}
	}
}

// checkQuickCancel calls the cancellation handler on c and fails unless it ends quickly.
// It returns the error returned by the
func checkQuickCancel(ctx context.Context, t *testing.T, c call.Connection) error {
	atomic.StoreInt64(&cancelCount, 0)
	start := time.Now()
	_, err := c.Call(ctx, cancelWaitKey, []byte("hello"), call.CallOptions{})
	elapsed := time.Since(start)
	t.Logf("ended with %v after %v", err, elapsed)

	if elapsed >= shortDelay+delaySlop {
		t.Errorf("call took %v even though we expected cancellation in %v",
			elapsed, shortDelay)
	}

	waitUntil(t, func() bool { return atomic.LoadInt64(&cancelCount) == 1 })
	return err
}

func testCall(ctx context.Context, t *testing.T, client call.Connection) {
	const arg = "hello"
	result, err := client.Call(ctx, echoKey, []byte(arg), call.CallOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r := string(result); r != arg {
		t.Fatalf("bad result: %q, expecting %q", r, arg)
	}
}

func testConcurrentCalls(t *testing.T, client call.Connection) {
	ctx, cancel := context.WithTimeout(context.Background(), shortDelay)
	defer cancel()

	caller := func(ctx context.Context, i int) error {
		for ctx.Err() == nil {
			arg := strconv.Itoa(i)
			result, err := client.Call(ctx, echoKey, []byte(arg), call.CallOptions{})
			if err != nil && errors.Is(err, ctx.Err()) {
				return nil
			} else if err != nil {
				return fmt.Errorf("unexpected error: %v", err)
			}
			if r := string(result); r != arg {
				return fmt.Errorf("bad result: %q, expecting %q", r, arg)
			}
		}
		return nil
	}

	numCallers := 10
	errs := make(chan error, numCallers)
	for i := 0; i < numCallers; i++ {
		i := i
		go func() { errs <- caller(ctx, i) }()
	}

	timer := time.NewTimer(testTimeout)
	for i := 0; i < numCallers; i++ {
		select {
		case err := <-errs:
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

		case <-timer.C:
			t.Fatal("test timed out")
		}
	}
}

func testError(t *testing.T, client call.Connection) {
	const msg = "error-message"
	_, err := client.Call(context.Background(), errorKey, []byte(msg), call.CallOptions{})
	if err == nil {
		t.Fatal("unexpected success when expecting error")
	}
	if r := err.Error(); !strings.Contains(r, msg) {
		t.Fatalf("unexpected error %q, expecting %q", r, msg)
	}
	if !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("bad error %v; does not match os.ErrInvalid", err)
	}
	if errors.Is(err, os.ErrPermission) {
		t.Fatalf("bad error %v; unexpectedly matches os.ErrPermission", err)
	}
}

func testDeadlineHandling(t *testing.T, client call.Connection) {
	// Test cancellation and deadline expiration.
	for _, useDeadline := range []bool{false, true} {
		t.Run(fmt.Sprintf("deadline=%v", useDeadline), func(t *testing.T) {
			// Run with a context that will get canceled shortly.
			ctx := context.Background()
			if useDeadline {
				c, cancelFunc := context.WithDeadline(ctx, time.Now().Add(shortDelay))
				defer cancelFunc()
				ctx = c
			} else {
				c, cancel := context.WithCancel(context.Background())
				go func() {
					time.Sleep(shortDelay)
					cancel()
				}()
				ctx = c
			}

			checkQuickCancel(ctx, t, client)
		})
	}
}

func testClose(t *testing.T, client call.Connection) {
	ctx := context.Background()

	// Test that Close cancels pending calls.
	go func() {
		time.Sleep(shortDelay)
		client.Close()
	}()
	checkQuickCancel(ctx, t, client)

	// Test that calls fail on a closed connection.
	_, err := client.Call(ctx, echoKey, []byte{}, call.CallOptions{})
	if err == nil {
		t.Fatal("unexpected success when expecting error")
	}
	if got, want := err.Error(), "closed"; !strings.Contains(got, want) {
		t.Fatalf("unexpected error: got %q, want %q", got, want)
	}

	// Test that close can be called multiple times without panicking.
	for i := 0; i < 10; i++ {
		client.Close()
	}
}

// TestSingleTCPServer runs a set of tests against a single TCP server. This
// test tests that a server can serve multiple clients, and that clients don't
// interfere with each other (e.g., closing one client doesn't affect other
// clients).
func TestSingleTCPServer(t *testing.T) {
	subtests := []struct {
		name string
		f    func(*testing.T, call.Connection)
	}{
		{"TestCall", func(t *testing.T, c call.Connection) { testCall(context.Background(), t, c) }},
		{"TestConcurrentCalls", testConcurrentCalls},
		{"TestError", testError},
		{"TestDeadlineHandling", testDeadlineHandling},
		// Note that testClose has to come last because once the connection is
		// closed, all other operations will fail.
		{"TestClose", testClose},
	}

	protocols := []string{"tcp", "mtls"}

	ctx := context.Background()
	opts := call.ServerOptions{Logger: logger(t)}
	endpoints := startServers(ctx, opts)

	// Run all of the subtests on a single connection.
	for resolverName, maker := range resolverMakers {
		for _, protocol := range protocols {
			client := getClientConn(t, protocol, endpoints[protocol], maker)
			for _, subtest := range subtests {
				name := fmt.Sprintf("Shared/%s/%s/%s", resolverName, protocol, subtest.name)
				t.Run(name, func(t *testing.T) { subtest.f(t, client) })
			}
			client.Close()
		}
	}

	// Run all of the subtests on a fresh connection.
	for resolverName, maker := range resolverMakers {
		for _, protocol := range protocols {
			for _, subtest := range subtests {
				name := fmt.Sprintf("Fresh/%s/%s/%s", resolverName, protocol, subtest.name)
				client := getClientConn(t, protocol, endpoints[protocol], maker)
				t.Run(name, func(t *testing.T) { subtest.f(t, client) })
				client.Close()
			}
		}
	}
}

// TestTracePropagation tests that the trace context is propagated across an RPC
func TestTracePropagation(t *testing.T) {
	ct := startTest(t)
	client := ct.connect(call.NewConstantResolver(ct.startTCPServer()))

	ctx, span := traceio.TestTracer().Start(context.Background(), "test")
	defer span.End()

	expect := span.SpanContext().WithRemote(true)
	_, err := runAtServer(ctx, client, call.CallOptions{}, func(ctx context.Context) ([]byte, error) {
		span := trace.SpanFromContext(ctx)
		if !span.SpanContext().IsValid() {
			return nil, fmt.Errorf("invalid span")
		}
		if expect.TraceID() != span.SpanContext().TraceID() {
			return nil, fmt.Errorf("unexpected trace id")
		}
		parent := span.(sdktrace.ReadOnlySpan).Parent()
		if !expect.Equal(parent) {
			want, _ := json.Marshal(expect)
			got, _ := json.Marshal(parent)
			return nil, fmt.Errorf("span context diff, want %q, got %q", want, got)
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestMultipleEndpoints tests that RPC calls succeed when the resolver returns
// a constant set of multiple endpoints.
func TestMultipleEndpoints(t *testing.T) {
	for name, maker := range resolverMakers {
		t.Run(name, func(t *testing.T) {
			n := 3
			ctx := context.Background()
			resolver := maker(server(t, "0"), server(t, "1"), server(t, "2"))
			options := call.ClientOptions{
				Balancer: call.RoundRobin(),
				Logger:   logger(t),
			}
			client, err := call.Connect(ctx, resolver, options)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()

			// Run a bunch of calls and check that they spread out over the replicas.
			count := map[string]int{}
			const attempts = 100
			for i := 0; i < attempts; i++ {
				result, err := client.Call(ctx, whoKey, []byte{}, call.CallOptions{})
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				count[string(result)]++
			}
			want := attempts / n
			for _, k := range []string{"0", "1", "2"} {
				got := count[k]
				if got < want/2 || got > want*2 {
					t.Errorf("replica %s got %d, expecting ~%d", k, got, want)
				}
			}
		})
	}
}

// TestChangingEndpoints tests that RPC calls succeed across endpoint changes.
func TestChangingEndpoints(t *testing.T) {
	n := 3
	ctx := context.Background()
	resolver := newDynamicResolver()
	opts := call.ClientOptions{Logger: logger(t)}
	client, err := call.Connect(ctx, resolver, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer client.Close()

	for i := 0; i < n; i++ {
		name := strconv.Itoa(i)
		resolver.Endpoints(server(t, name))
		waitUntil(t, func() bool {
			result, err := client.Call(ctx, whoKey, []byte{}, call.CallOptions{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			return string(result) == name
		})
	}
}

// TestNoEndpointsConstant tests that it is an error to call Connect with a
// constant resolver that returns no endpoints.
func TestNoEndpointsConstant(t *testing.T) {
	ctx := context.Background()
	opts := call.ClientOptions{Logger: logger(t)}
	_, err := call.Connect(ctx, call.NewConstantResolver(), opts)
	if err == nil {
		t.Fatal("unexpected success when expecting error")
	}
	if got, want := err, call.Unreachable; !errors.Is(got, want) {
		t.Fatalf("bad error: got %v, want %v", got, want)
	}
}

// TestNoEndpointsNonConstant tests that it is not an error to call Connect
// with a non-constant resolver that returns no endpoints, but it is an error
// to make a call when there are no endpoints.
func TestNoEndpointsNonConstant(t *testing.T) {
	ctx := context.Background()
	resolver := newDynamicResolver()

	// Connecting with a non-constant resolver isn't an error because the
	// resolver may return endpoints later.
	opts := call.ClientOptions{Logger: logger(t)}
	client, err := call.Connect(ctx, resolver, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Making a call without any endpoints is an error though.
	sub, cancel := context.WithTimeout(ctx, shortDelay)
	_, err = client.Call(sub, echoKey, []byte{}, call.CallOptions{})
	cancel()
	if got, want := err, context.DeadlineExceeded; !errors.Is(got, want) {
		t.Fatalf("bad error: got %v, want %v", got, want)
	}

	// Add an endpoint and let the update propagate.
	resolver.Endpoints(server(t, "server"))
	waitUntil(t, func() bool {
		sub, cancel := context.WithTimeout(ctx, shortDelay)
		defer cancel()
		_, err = client.Call(sub, echoKey, []byte{}, call.CallOptions{})
		return err == nil
	})

	// Remove the endpoint.
	resolver.Endpoints()
	waitUntil(t, func() bool {
		sub, cancel := context.WithTimeout(ctx, shortDelay)
		defer cancel()
		_, err = client.Call(sub, echoKey, []byte{}, call.CallOptions{})
		return errors.Is(err, context.DeadlineExceeded)
	})
}

// TestEndpointsRetained tests that connections are retained across endpoint
// changes. For example, if a resolver returns endpoints {a, b} and then later
// {b, c}, the connection to b should be retained.
func TestEndpointsRetained(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Construct the network.
	c, s := pipe(t)
	sopts := call.ServerOptions{Logger: logger(t)}
	call.ServeOn(ctx, s, handlersFor("1"), sopts)
	m := &closeMock{connWrapper: connWrapper{c}}
	server1 := &connEndpoint{"1", m}
	server2 := server(t, "2")

	// Construct the client.
	resolver := newDynamicResolver(server1)
	copts := call.ClientOptions{Logger: logger(t)}
	client, err := call.Connect(ctx, resolver, copts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer client.Close()

	// Make a call to establish a connection with server 1.
	if _, err := client.Call(ctx, echoKey, []byte{}, call.CallOptions{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Update the endpoints, but retain server 1.
	resolver.Endpoints(server1, server2)
	waitUntil(t, func() bool {
		result, err := client.Call(ctx, whoKey, []byte{}, call.CallOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return string(result) == "2"
	})
	if m.Closed() {
		t.Fatal("client unexpectedly closed")
	}
}

// TestDraining tests that pending RPCs on a draining connection are allowed to
// finish. Once the RPCs have completed, the draining connection should close.
func TestDraining(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Construct the network.
	c, s := pipe(t)
	sopts := call.ServerOptions{Logger: logger(t)}
	call.ServeOn(ctx, s, handlersFor("1"), sopts)
	m := &closeMock{connWrapper: connWrapper{c}}
	server1 := &connEndpoint{"1", m}
	server2 := server(t, "2")

	// Construct the client.
	resolver := newDynamicResolver(server1)
	copts := call.ClientOptions{Logger: logger(t)}
	client, err := call.Connect(ctx, resolver, copts)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Launch a separate goroutine to update the endpoints from server 1 to
	// server 2. This makes server 1 stale.
	go func() {
		time.Sleep(shortDelay)
		resolver.Endpoints(server2)
	}()

	// Launch the caller goroutines.
	caller := func() error {
		// This call should last long enough for the connection to become
		// stale, but it should still succeed.
		_, err := client.Call(ctx, sleepKey, []byte(delaySlop.String()), call.CallOptions{})
		return err
	}
	numCallers := 10
	errs := make(chan error, numCallers)
	for i := 0; i < numCallers; i++ {
		go func() { errs <- caller() }()
	}

	// Wait for the goroutines to finish.
	timer := time.NewTimer(testTimeout)
	for i := 0; i < numCallers; i++ {
		select {
		case err := <-errs:
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

		case <-timer.C:
			t.Fatal("test timed out")
		}
	}

	// Make sure the connection was closed.
	if !m.Closed() {
		t.Fatalf("drained connection not closed")
	}
}

// TestNoActiveDraining tests that an draining connection with no active calls
// is closed immediately.
func TestNoActiveDraining(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Construct the network.
	c, s := pipe(t)
	sopts := call.ServerOptions{Logger: logger(t)}
	call.ServeOn(ctx, s, handlersFor("1"), sopts)
	m := &closeMock{connWrapper: connWrapper{c}}
	server1 := &connEndpoint{"1", m}
	server2 := server(t, "2")

	// Construct the client.
	resolver := newDynamicResolver(server1)
	copts := call.ClientOptions{Logger: logger(t)}
	client, err := call.Connect(ctx, resolver, copts)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Make a call to establish a connection with server 1.
	if _, err = client.Call(ctx, echoKey, []byte{}, call.CallOptions{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Update the endpoints from server 1 to server 2. This makes server 1
	// stale.
	resolver.Endpoints(server2)
	waitUntil(t, func() bool {
		result, err := client.Call(ctx, whoKey, []byte{}, call.CallOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return string(result) == "2"
	})

	// Make sure the connection was closed.
	if !m.Closed() {
		t.Fatalf("drained connection not closed")
	}
}

// TestCloseDraining tests that Close closes draining connections.
func TestCloseDraining(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Construct the network.
	c, s := pipe(t)
	sopts := call.ServerOptions{Logger: logger(t)}
	call.ServeOn(ctx, s, handlersFor("1"), sopts)
	m := &closeMock{connWrapper: connWrapper{c}}
	server1 := &connEndpoint{"1", m}
	server2 := server(t, "2")

	// Construct the client.
	resolver := newDynamicResolver(server1)
	copts := call.ClientOptions{Logger: logger(t)}
	client, err := call.Connect(ctx, resolver, copts)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Launch a separate goroutine to update the endpoints from server 1 to
	// server 2. This makes server 1 stale. Then, close the client after a
	// delay.
	go func() {
		time.Sleep(shortDelay)
		resolver.Endpoints(server2)
		time.Sleep(delaySlop) // Let the update propagate.
		client.Close()
	}()

	// Call the sleepHandler. The call should last long enough for the
	// connection to become stale. It will fail when the client is closed.
	if _, err := client.Call(ctx, sleepKey, []byte(testTimeout.String()), call.CallOptions{}); !errors.Is(err, call.CommunicationError) {
		t.Fatalf("bad error: got %v, want %v", err, call.CommunicationError)
	}

	// Make sure the connection is closed soon.
	time.Sleep(shortDelay)
	if !m.Closed() {
		t.Fatalf("draining connection not closed")
	}
}

// TestRememberDraining tests that all draining connections are maintained, not
// just the most recent set of stale connections.
func TestRememberDraining(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Construct the network.
	c, s := pipe(t)
	sopts := call.ServerOptions{Logger: logger(t)}
	call.ServeOn(ctx, s, handlers, sopts)
	m := &closeMock{connWrapper: connWrapper{c}}
	server1 := &connEndpoint{"1", m}
	server2 := server(t, "2")
	server3 := server(t, "3")

	// Construct the client.
	resolver := newDynamicResolver(server1)
	copts := call.ClientOptions{Logger: logger(t)}
	client, err := call.Connect(ctx, resolver, copts)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Launch a separate goroutine to update the endpoints from server 1 to
	// server 2 to server 3. This makes server 1 and 2 stale. Then, close the
	// client after a delay.
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		time.Sleep(shortDelay)
		resolver.Endpoints(server2)
		time.Sleep(delaySlop) // Let the update propagate.
		resolver.Endpoints(server3)
		time.Sleep(delaySlop) // Let the update propagate.
		client.Close()
	}()

	// Call the sleepHandler. The call should last long enough for the
	// connection to become stale. It will fail when the client is closed.
	if _, err := client.Call(ctx, sleepKey, []byte(testTimeout.String()), call.CallOptions{}); !errors.Is(err, call.CommunicationError) {
		t.Fatalf("bad error: got %v, want %v", err, call.CommunicationError)
	}

	// Wait for client.Close() to terminate to ensure all connections have had
	// a chance to get closed.
	wait.Wait()

	// Make sure the connection is closed.
	if !m.Closed() {
		t.Fatalf("draining connection not closed")
	}
}

// TestRefreshDraining tests that a draining connection can be restored to a
// non-draining connection.
func TestRefreshDraining(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Construct the network.
	c, s := pipe(t)
	sopts := call.ServerOptions{Logger: logger(t)}
	call.ServeOn(ctx, s, handlersFor("1"), sopts)
	m := &closeMock{connWrapper: connWrapper{c}}
	server1 := &connEndpoint{"1", m}
	server2 := server(t, "2")

	// Construct the client.
	resolver := newDynamicResolver(server1)
	copts := call.ClientOptions{Logger: logger(t)}
	client, err := call.Connect(ctx, resolver, copts)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Launch a separate goroutine to update the endpoints from server 1 to
	// server 2 and then back to server 1. This makes server 1 stale and then
	// refreshed.
	go func() {
		time.Sleep(shortDelay)
		resolver.Endpoints(server2)
		time.Sleep(delaySlop) // Let the update propagate.
		resolver.Endpoints(server1)
	}()

	// Call the sleepHandler. The call should last long enough for the
	// connection to become stale and then refreshed.
	if _, err := client.Call(ctx, sleepKey, []byte((2 * delaySlop).String()), call.CallOptions{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestUnhealthyChannels attempts to use a resolver that returns a mix of
// healthy and unhealthy endpoints and verifies that calls succeed by being
// routed to the healthy backend.
func TestInitiallyUnhealthyEndpoint(t *testing.T) {
	// To fix this, we will avoid connections on which an initial
	// health-check has not completed. There are some options here:
	//
	// 1. We let the balancer Pick() stay as it is currently where it does
	// not know about connections, just endpoints. After a Pick(), if the
	// connection does not exist or the health-check hasn't completed, we
	// Pick() from the subset of connections with healthy endpoints.
	//
	// 2. We eagerly connect to any endpoint handed to us by the Resolver.
	// Balancer is passed a connection only after the initial hand-shake
	// has completed. Balancer.Pick() returns a connection, not an
	// endpoint.
	//
	// 3. We return a special error to startCall if we are attempting to
	// use a connection that hasn't completed its initial health
	// check. startCall retries after a delay if it sees this error.
	//
	// Option 2 seems the best? Its main potential downside is that we may
	// make too many connections. However we can avoid having too many
	// connections when there are many clients and many servers by
	// implementing backend subsetting inside the Resolver.

	// Start a good server and a bad server.
	ct := startTest(t)
	s1, s2 := ct.startTCPServer(), ct.startTCPServer()
	bad, good := hangingEndpoint{s1}, s2
	resolver := call.NewConstantResolver(bad, good)

	// Connect via a balancer whose picking decision we control.
	var useGood atomic.Bool
	balancer := call.BalancerFunc(func(conns []call.ReplicaConnection, options call.CallOptions) (call.ReplicaConnection, bool) {
		want := bad.Address()
		if useGood.Load() {
			want = good.Address()
		}
		for _, c := range conns {
			if c.Address() == want {
				return c, true
			}
		}
		return nil, false
	})
	client, err := call.Connect(ct.ctx, resolver, call.ClientOptions{
		Balancer: balancer,
		Logger:   logger(t),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Switch to using the good endpoint after a delay.
	ct.fork(func() {
		time.Sleep(shortDelay)
		useGood.Store(true)
	})

	start := time.Now()
	testCall(ct.ctx, t, client)
	elapsed := time.Since(start)
	if elapsed < shortDelay {
		t.Fatalf("call completed too soon: after %v, expecting at least %v", elapsed, shortDelay)
	}
	if elapsed >= 4*shortDelay {
		t.Fatalf("call completed too late: after %v, expecting approximately %v", elapsed, shortDelay)
	}
}

func TestCommunicationErrors(t *testing.T) {
	for name, maker := range resolverMakers {
		t.Run(name, func(t *testing.T) {
			ctx, cancelFunc := context.WithDeadline(context.Background(), time.Now().Add(testTimeout))
			defer cancelFunc()

			c, s := pipe(t)
			sopts := call.ServerOptions{Logger: logger(t)}
			call.ServeOn(ctx, s, handlers, sopts)
			endpoint := &connEndpoint{"server", c}
			copts := call.ClientOptions{Logger: logger(t)}
			client, err := call.Connect(ctx, maker(endpoint), copts)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()

			// Inject error by closing the connection after a delay.
			go func() {
				time.Sleep(shortDelay)
				c.Close()
			}()

			err = checkQuickCancel(ctx, t, client)
			if err == nil || !errors.Is(err, call.CommunicationError) {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestWriteError(t *testing.T) {
	type subtest struct {
		name       string
		writeError string
		maker      resolverMaker
	}
	subtests := []subtest{}
	for resolverName, maker := range resolverMakers {
		for _, writeError := range []string{
			"WriteRequest",
			"WriteResponse",
			"ReadRequest",
			"ReadResponse",
		} {
			name := fmt.Sprintf("%s/%s", resolverName, writeError)
			subtests = append(subtests, subtest{name, writeError, maker})
		}
	}

	// Try with error injections on different paths.
	for _, test := range subtests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancelFunc := context.WithDeadline(context.Background(), time.Now().Add(testTimeout))
			defer cancelFunc()

			c, s := pipe(t)

			// Configure errors.
			const limit = 1000
			switch test.writeError {
			case "WriteRequest":
				c = &writeErrorInjector{connWrapper: connWrapper{c}, limit: limit}
			case "WriteResponse":
				s = &writeErrorInjector{connWrapper: connWrapper{s}, limit: limit}
			case "ReadRequest":
				s = &readErrorInjector{connWrapper: connWrapper{s}, limit: limit}
			case "ReadResponse":
				c = &readErrorInjector{connWrapper: connWrapper{c}, limit: limit}
			}

			sopts := call.ServerOptions{Logger: logger(t)}
			call.ServeOn(ctx, s, handlers, sopts)
			endpoint := &connEndpoint{"server", c}
			copts := call.ClientOptions{Logger: logger(t)}
			client, err := call.Connect(ctx, test.maker(endpoint), copts)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()

			// Start first call which should hang until error causes connection break.
			// This will use up some bytes from limit but won't exceed limit.
			var wg sync.WaitGroup
			wg.Add(1)
			var err1 error
			go func() {
				_, err1 = client.Call(ctx, cancelWaitKey, []byte("hello"), call.CallOptions{})
				wg.Done()
			}()
			time.Sleep(shortDelay)

			// Second call should encounter the injected error because it
			// needs more than limit bytes.
			_, err2 := client.Call(ctx, echoKey, make([]byte, limit), call.CallOptions{})

			wg.Wait()
			if !errors.Is(err1, call.CommunicationError) {
				t.Errorf("unexpected error: %v", err1)
			}
			if !errors.Is(err2, call.CommunicationError) {
				t.Errorf("unexpected error: %v", err2)
			}
		})
	}
}

func TestReconnect(t *testing.T) {
	for name, maker := range resolverMakers {
		t.Run(name, func(t *testing.T) {
			ctx, cancelFunc := context.WithDeadline(context.Background(), time.Now().Add(testTimeout))
			defer cancelFunc()

			// Make a list of connections to try, each of which will fail after limit bytes.
			const limit = 1000
			const count = 100
			conns := make([]net.Conn, count)
			for i := range conns {
				c, s := pipe(t)
				conns[i] = &writeErrorInjector{connWrapper: connWrapper{c}, limit: limit}
				sopts := call.ServerOptions{Logger: logger(t)}
				call.ServeOn(ctx, s, handlers, sopts)
			}

			// Make a client that uses the created connections in order.
			endpoint := &connsEndpoint{name: "server", conns: conns}
			copts := call.ClientOptions{Logger: logger(t)}
			client, err := call.Connect(ctx, maker(endpoint), copts)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()

			for i := 0; i < count; i++ {
				// Make a small call that does not exceed limit and should succeed.
				arg := fmt.Sprintf("short%d", i)
				res, err := client.Call(ctx, echoKey, []byte(arg), call.CallOptions{})
				if err != nil {
					t.Fatalf("unexpected error on short call %d: %v", i, err)
				}
				if string(res) != string(arg) {
					t.Fatalf("unexpected result %d: %q, expecting %q", i, string(res), string(arg))
				}

				// Make a large call that should fail due to exceeding limit.
				_, err = client.Call(ctx, echoKey, make([]byte, limit), call.CallOptions{})
				if !errors.Is(err, call.CommunicationError) {
					t.Fatalf("unexpected error on long call %d: %v", i, err)
				}
			}
		})
	}
}

func TestPartialFailure(t *testing.T) {
	for name, maker := range resolverMakers {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			// We use round-robin to balance between a dead and a live endpoint.
			server1 := &deadEndpoint{"1"}
			server2 := server(t, "2")

			resolver := maker(server1, server2)
			options := call.ClientOptions{
				Balancer: call.RoundRobin(),
				Logger:   logger(t),
			}
			client, err := call.Connect(ctx, resolver, options)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()

			// All calls should bypass the bad backend.
			const num = 4
			for i := 0; i < num; i++ {
				const arg = "hello"
				result, err := client.Call(ctx, echoKey, []byte(arg), call.CallOptions{})
				if err != nil {
					t.Fatalf("unexpected error %v", err)
				}
				if got, want := string(result), arg; got != want {
					t.Fatalf("bad result: got %q, want %q", got, want)
				}
			}
		})
	}
}

// TestManyEndpointChanges tests that we can issue concurrent RPC calls, even
// when the set of endpoints is changing frequently.
func TestManyEndpointChanges(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Construct the client.
	numServers := 10
	numChanges := 25
	numCallers := 5
	resolver := newDynamicResolver(servers(t, numServers)...)
	opts := call.ClientOptions{Logger: logger(t)}
	client, err := call.Connect(ctx, resolver, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	// Run the goroutines.
	updater := func(cancel context.CancelFunc) {
		for i := 0; i < numChanges; i++ {
			endpoints := servers(t, numServers)
			rand.Shuffle(len(endpoints), func(i, j int) {
				endpoints[i], endpoints[j] = endpoints[j], endpoints[i]
			})
			resolver.Endpoints(endpoints[:rand.Intn(len(endpoints))]...)
			time.Sleep(shortDelay)
		}

		// When the updater is done, it cancels the context, which will cause
		// the callers to exit.
		cancel()
	}

	caller := func(ctx context.Context, client call.Connection) error {
		for ctx.Err() == nil {
			const arg = "hello"
			_, err := client.Call(ctx, echoKey, []byte(arg), call.CallOptions{})
			if err != nil && errors.Is(err, ctx.Err()) {
				return nil
			} else if err != nil && errors.Is(err, call.Unreachable) {
				// We may get an Unreachable error if the resolver returned no
				// addresses. This is expected, so we ignore the error.
				continue
			} else if err != nil {
				return err
			}
		}
		return nil
	}

	go func() { updater(cancel) }()
	errs := make(chan error, numCallers)
	for i := 0; i < numCallers; i++ {
		go func() { errs <- caller(ctx, client) }()
	}

	// Wait for the goroutines to finish.
	timer := time.NewTimer(testTimeout)
	for i := 0; i < numCallers; i++ {
		select {
		case err := <-errs:
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

		case <-timer.C:
			t.Fatal("test timed out")
		}
	}
}

// TestResolverError tests that when Resolve returns a non-nil error, we try
// again with exponential backoff, rather than trying again right away.
func TestResolverError(t *testing.T) {
	// Create the client.
	ctx := context.Background()
	resolver := &failResolver{}
	opts := call.ClientOptions{Logger: logger(t)}
	client, err := call.Connect(ctx, resolver, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	time.Sleep(delaySlop)

	if n := resolver.Get(); n > 10000 {
		t.Fatalf("Resolve called too many times: %d times", n)
	}
}

func TestNoRetry(t *testing.T) {
	ct := startTest(t)
	client := ct.connect(call.NewConstantResolver(ct.startTCPServer()))

	type testCase struct {
		name   string // Test case name
		inject error  // Error to return from server side
	}
	for _, c := range []testCase{
		{"success", nil},
		{"communication", call.CommunicationError},
		{"unreachable", call.CommunicationError},
		{"non-retriable-error", os.ErrInvalid},
	} {
		c := c
		t.Run(c.name, func(t *testing.T) {
			_, err := runAtServer(ct.ctx, client, call.CallOptions{Retry: false}, func(context.Context) ([]byte, error) {
				return nil, c.inject
			})
			if !errors.Is(err, c.inject) {
				t.Fatalf("got %v calls, expecting %v", err, c.inject)
			}
		})
	}
}

func TestRetry(t *testing.T) {
	ct := startTest(t)
	client := ct.connect(call.NewConstantResolver(ct.startTCPServer()))

	// Use a server handler that returns a specified sequence of errors.
	type testCase struct {
		name          string  // Test case name
		errors        []error // Sequence of errors to return from repeated calls
		expectedCalls int     // Number of expected calls
	}
	for _, c := range []testCase{
		{"success", []error{nil, os.ErrInvalid}, 1},
		{"eventual-success", []error{call.CommunicationError, nil, os.ErrInvalid}, 2},
		{"communication", []error{call.CommunicationError, os.ErrInvalid}, 2},
		{"unreachable", []error{call.Unreachable, os.ErrInvalid}, 2},
		{"non-retriable-error", []error{os.ErrPermission, os.ErrInvalid}, 1},
	} {
		c := c
		t.Run(c.name, func(t *testing.T) {
			var count atomic.Int32
			_, err := runAtServer(ct.ctx, client, call.CallOptions{Retry: true}, func(context.Context) ([]byte, error) {
				i := int(count.Add(1)) - 1
				if i >= len(c.errors) {
					i = len(c.errors) - 1 // Return last error repeatedly
				}
				return nil, c.errors[i]
			})
			t.Logf("got %v after %d calls", err, count.Load())
			n := int(count.Load())
			if n != c.expectedCalls {
				t.Fatalf("got %d calls, expecting %d", n, c.expectedCalls)
			}
			if !errors.Is(err, c.errors[c.expectedCalls-1]) {
				t.Fatalf("got %v after %d calls, expecting %v", err, count.Load(), c.errors[n-1])
			}
		})
	}
}

func BenchmarkCall(b *testing.B) {
	ctx := context.Background()
	opts := call.ServerOptions{Logger: logger(b)}
	endpoints := startServers(ctx, opts)

	for resolverName, maker := range resolverMakers {
		for _, protocol := range []string{"tcp"} {
			client := getClientConn(b, protocol, endpoints[protocol], maker)
			ctx := context.Background()
			for _, msgSize := range []int{1, 65536, 1048576} {
				b.Run(fmt.Sprintf("%s/%s/Msg-%s", resolverName, protocol, sizeString(msgSize)), func(b *testing.B) {
					msg := make([]byte, msgSize)
					for i := range msg {
						msg[i] = 'x'
					}
					for i := 0; i < b.N; i++ {
						result, err := client.Call(ctx, echoKey, msg, call.CallOptions{})
						if err != nil {
							b.Fatal(err)
						}
						if len(result) != len(msg) {
							b.Fatalf("wrong length %d; expecting %d", len(result), len(msg))
						}
					}
				})
			}
		}
	}
}

func TestCancelServe(t *testing.T) {
	// Check that a server stops quickly when its context is canceled.
	ctx, cancelFunc := context.WithCancel(context.Background())

	// Run server in the background.
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		opts := call.ServerOptions{Logger: logger(t)}
		err := call.Serve(ctx, testListener{Listener: lis}, opts)
		if err != ctx.Err() {
			t.Errorf("unexpected error from Serve: %v", err)
		}
		close(done)
	}()

	// Cancel after a delay and check that the server stops quickly.
	time.Sleep(shortDelay)
	cancelFunc()
	start := time.Now()
	select {
	case <-done:
		// Stopped.
	case <-time.After(shortDelay + delaySlop):
		t.Fatal("cancellation timed out after", time.Since(start))
	}
}

// failResolver is a resolver with a Resolve method that always fails after the
// first time it's called.
type failResolver struct {
	mu sync.Mutex // guards access to n
	n  int        // the number of times Resolve has been called
}

var _ call.Resolver = &failResolver{}

// IsConstant implements the call.Resolver interface.
func (f *failResolver) IsConstant() bool {
	return false
}

// Resolve implements the call.Resolver interface.
func (f *failResolver) Resolve(context.Context, *call.Version) ([]call.Endpoint, *call.Version, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n += 1
	if f.n == 1 {
		return []call.Endpoint{}, &call.Version{}, nil
	}
	return nil, nil, fmt.Errorf("injected resolver error")
}

// Get returns the number of times Resolve was called.
func (f *failResolver) Get() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n
}

// dynamicResolver is a non-constant Resolver testing stub.
type dynamicResolver struct {
	m         sync.Mutex      // guards all of the following fields
	changed   cond.Cond       // fires when endpoints changes
	version   int             // the current version of endpoints
	endpoints []call.Endpoint // the endpoints returned by Resolve
}

// Check that DynamicResolver implements the Resolver interface.
var _ call.Resolver = &dynamicResolver{}

// newDynamicResolver returns a new dynamicResolver that returns the provided
// addresses. To update the returned addresses, use the Endpoints method.
func newDynamicResolver(endpoints ...call.Endpoint) *dynamicResolver {
	r := dynamicResolver{endpoints: endpoints}
	r.changed.L = &r.m
	return &r
}

// getVersion returns the version of the current set of addresses.
func (d *dynamicResolver) getVersion() *call.Version {
	return &call.Version{Opaque: strconv.Itoa(d.version)}
}

// Endpoints updates the set of endpoints that the resolver returns.
func (d *dynamicResolver) Endpoints(endpoints ...call.Endpoint) {
	d.m.Lock()
	defer d.m.Unlock()
	d.version += 1
	d.endpoints = endpoints
	d.changed.Broadcast()
}

// IsConstant implements the Resolver interface.
func (*dynamicResolver) IsConstant() bool {
	return false
}

// Resolve implements the Resolver interface.
func (d *dynamicResolver) Resolve(ctx context.Context, version *call.Version) ([]call.Endpoint, *call.Version, error) {
	d.m.Lock()
	defer d.m.Unlock()

	if version == nil {
		return d.endpoints, d.getVersion(), nil
	}

	for *version == *d.getVersion() {
		if err := d.changed.Wait(ctx); err != nil {
			return nil, nil, err
		}
	}
	return d.endpoints, d.getVersion(), nil
}

// connWrapper wraps the public API of net.Conn.
//
// We do not directly embed net.Conn since that would cause
// net.Buffers to directly call internal APIs and bypass any injection
// we might want to do in our tests.
type connWrapper struct{ c net.Conn }

func (w *connWrapper) Close() error                       { return w.c.Close() }
func (w *connWrapper) LocalAddr() net.Addr                { return w.c.LocalAddr() }
func (w *connWrapper) RemoteAddr() net.Addr               { return w.c.RemoteAddr() }
func (w *connWrapper) SetDeadline(t time.Time) error      { return w.c.SetDeadline(t) }
func (w *connWrapper) SetReadDeadline(t time.Time) error  { return w.c.SetReadDeadline(t) }
func (w *connWrapper) SetWriteDeadline(t time.Time) error { return w.c.SetWriteDeadline(t) }
func (w *connWrapper) Read(b []byte) (int, error)         { return w.c.Read(b) }
func (w *connWrapper) Write(b []byte) (int, error)        { return w.c.Write(b) }

// writeErrorInjector injects an error on writes after some number of bytes are written.
type writeErrorInjector struct {
	connWrapper
	mu    sync.Mutex
	limit int
}

var _ net.Conn = &writeErrorInjector{}

func (c *writeErrorInjector) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.limit <= len(b) {
		return 0, fmt.Errorf("injected write error")
	}
	n, err := c.connWrapper.Write(b)
	c.limit -= n
	return n, err
}

// readErrorInjector injects an error on writes after some number of bytes are read.
type readErrorInjector struct {
	connWrapper
	mu    sync.Mutex
	limit int
}

var _ net.Conn = &readErrorInjector{}

func (c *readErrorInjector) Read(b []byte) (int, error) {
	n, err := c.connWrapper.Read(b)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.limit -= n
	if err != nil {
		return n, err
	}
	if c.limit <= 0 {
		return 0, fmt.Errorf("injected read error")
	}
	return n, nil
}

// closeMock records whether Close was called.
type closeMock struct {
	connWrapper
	mu     sync.Mutex
	closed bool
}

var _ net.Conn = &closeMock{}

func (c *closeMock) Close() error {
	err := c.connWrapper.Close()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return err
}

func (c *closeMock) Closed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func sizeString(s int) string {
	if s >= 1048576 {
		return fmt.Sprintf("%gM", float64(s)/1048576)
	}
	if s >= 1024 {
		return fmt.Sprintf("%gK", float64(s)/1024)
	}
	return fmt.Sprint(s)
}

func logger(t testing.TB) *slog.Logger {
	return logging.NewTestSlogger(t, testing.Verbose())
}
