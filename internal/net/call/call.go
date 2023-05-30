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

// Package call implements an RPC mechanism.
package call

// # Overview
//
// RPCs are conveyed across a bidirectional connection. A connection carries
// a sequence of messages in each direction. A message has the following
// information:
//	request-id	-- A number that identifies a particular RPC
//	message-type	-- E.g., request or response
//	length		-- How many payload bytes follow
//	payload		-- length bytes of payload
// The payload format varies depending on the message-type.
// See msg.go for details.
//
// # Server operation
//
// The server listens for connections (typically on a TCP socket). For
// each accepted connection, it starts a readRequests() goroutine that
// reads messages from that connection. When readRequests() gets a
// request message, it starts a runHandler() goroutine. runHandler()
// looks up the registered handler for the message, runs it, and sends
// the response back over the connection.
//
// # Client operation
//
// A client creates connections to one or more servers and, for every
// connection, starts a background readResponses() goroutine that reads
// messages from the connection.
//
// When the client wants to send an RPC, it selects one of its server
// connections to use, creates a call component, assigns it a new request-id, and
// registers the components in a map in the connection. It then sends a request
// message over the connection and waits for the call component to be marked as
// done.
//
// When the response arrives, it is picked up by readResponses().
// readResponses() finds the call component corresponding to the
// request-id in the response, and marks the call component as done which
// wakes up goroutine that initiated the RPC.
//
// If a client is constructed with a non-constant resolver, the client also
// spawns a watchResolver goroutine that repeatedly calls Resolve on the
// resolver to get notified of updates to the set of endpoints. When the
// endpoints are updated, existing connections are retained, and stale
// connections are transitioned to a "draining" state.
//
// New RPCs are never issued over draining connections, but the pending
// requests on a draining connection are allowed to finish. As soon as a
// draining connection has no active calls, the connection closes itself. If
// the resolver later returns a new set of endpoints that includes a draining
// connection that hasn't closed itself, the connection is transitioned out of
// the draining phase and is once again allowed to process new RPCs.

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/retry"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/slog"
)

const (
	// Size of the header included in each message.
	msgHeaderSize = 16 + 8 + traceHeaderLen // handler_key + deadline + trace_context

	// maxReconnectTries is the maximum number of times a reconnecting
	// connection will try and create a connection before erroring out.
	maxReconnectTries = 3
)

// TODO:
// - Preserve error types (maybe via registration and Gob?)
// - Load balancer
//   - API to allow changes to set
//   - health-checks
//   - track subset that is healthy
//   - track load info
//   - data structure for efficient picking (randomize? weighted?)
//   - pick on call (error if none available)

// Connection allows a client to send RPCs.
type Connection interface {
	// Call makes an RPC over a Connection.
	Call(context.Context, MethodKey, []byte, CallOptions) ([]byte, error)

	// Close closes a connection. Pending invocations of Call are cancelled and
	// return an error. All future invocations of Call fail and return an error
	// immediately. Close can be called more than once.
	Close()
}

// Listener allows the server to accept RPCs.
type Listener interface {
	Accept() (net.Conn, *HandlerMap, error)
	Close() error
	Addr() net.Addr
}

// reconnectingConnection is the concrete client-side Connection implementation.
// It automatically reconnects to the servers on first call or the first call
// after a shutdown.
type reconnectingConnection struct {
	opts ClientOptions

	// mu guards the following fields and some of the fields in the
	// clientConnections inside connections and draining.
	mu          sync.Mutex
	endpoints   []Endpoint
	connections map[string]*clientConnection // keys are endpoint addresses
	draining    map[string]*clientConnection // keys are endpoint addresses
	closed      bool

	resolver       Resolver
	cancelResolver func()         // cancels the watchResolver goroutine
	resolverDone   sync.WaitGroup // used to wait for watchResolver to finish
}

// clientConnection manages one network connection on the client-side.
type clientConnection struct {
	logger         *slog.Logger
	endpoint       Endpoint
	c              net.Conn
	cbuf           *bufio.Reader    // Buffered reader wrapped around c
	wlock          sync.Mutex       // Guards writes to c
	mu             *sync.Mutex      // Same as reconnectingConnection.mu
	draining       bool             // is this clientConnection draining?
	ended          bool             // has this clientConnection ended?
	loggedShutdown bool             // Have we logged a shutdown error?
	version        version          // Version number to use for connection
	calls          map[uint64]*call // In-progress calls
	lastID         uint64           // Last assigned request ID for a call
}

// call holds the state for an active call at the client.
type call struct {
	id         uint64
	doneSignal chan struct{}

	// Fields below are accessed across goroutines, but their access is
	// synchronized via doneSignal, i.e., it is never concurrent.
	err      error
	response []byte

	// Is the call done?
	// This field is accessed across goroutines using atomics.
	done uint32 // is the call done?

}

// serverConnection manages one network connection on the server-side.
type serverConnection struct {
	opts        ServerOptions
	c           net.Conn
	cbuf        *bufio.Reader // Buffered reader wrapped around c
	wlock       sync.Mutex    // Guards writes to c
	mu          sync.Mutex
	closed      bool              // has c been closed?
	version     version           // Version number to use for connection
	cancelFuncs map[uint64]func() // Cancellation functions for in-progress calls
}

// serverState tracks all live server-side connections so we can clean things up when canceled.
type serverState struct {
	opts  ServerOptions
	mu    sync.Mutex
	conns map[*serverConnection]struct{} // Live connections
}

// Serve starts listening for connections and requests on l. It always returns a
// non-nil error and closes l.
func Serve(ctx context.Context, l Listener, opts ServerOptions) error {
	opts = opts.withDefaults()
	ss := &serverState{opts: opts}
	defer ss.stop()
	l = &onceCloseListener{Listener: l}

	// Arrange to close the listener when the context is canceled.
	go func() {
		<-ctx.Done()
		l.Close()
	}()

	for {
		conn, hmap, err := l.Accept()
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case err != nil:
			l.Close()
			return fmt.Errorf("call server error listening on %s: %w", l.Addr(), err)
		}
		ss.serveConnection(ctx, conn, hmap)
	}
}

// onceCloseListener wraps a Listener, protecting it from multiple Close calls.
// TODO: replace with sync.OnceValues which should be available in go1.21
type onceCloseListener struct {
	Listener
	once     sync.Once
	closeErr error
}

func (oc *onceCloseListener) Close() error {
	oc.once.Do(func() {
		oc.closeErr = oc.Listener.Close()
	})
	return oc.closeErr
}

// ServeOn serves client requests received over an already established
// network connection with a client. This can be useful in tests or
// when using custom networking transports.
func ServeOn(ctx context.Context, conn net.Conn, hmap *HandlerMap, opts ServerOptions) {
	ss := &serverState{opts: opts.withDefaults()}
	ss.serveConnection(ctx, conn, hmap)
}

func (ss *serverState) serveConnection(ctx context.Context, conn net.Conn, hmap *HandlerMap) {
	c := &serverConnection{
		opts:        ss.opts,
		c:           conn,
		cbuf:        bufio.NewReader(conn),
		version:     initialVersion, // Updated when we hear from client
		cancelFuncs: map[uint64]func(){},
	}
	ss.register(c)

	go c.readRequests(ctx, hmap, func() { ss.unregister(c) })
}

func (ss *serverState) stop() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	for c := range ss.conns {
		c.c.Close() // Should stop readRequests in its tracks
	}
}

func (ss *serverState) register(c *serverConnection) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.conns == nil {
		ss.conns = map[*serverConnection]struct{}{}
	}
	ss.conns[c] = struct{}{}
}

func (ss *serverState) unregister(c *serverConnection) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	delete(ss.conns, c)
}

// Connect creates a connection to the servers at the endpoints returned by the
// resolver.
func Connect(ctx context.Context, resolver Resolver, opts ClientOptions) (Connection, error) {
	// Construct the connection.
	conn := reconnectingConnection{
		opts:           opts.withDefaults(),
		endpoints:      []Endpoint{},
		connections:    map[string]*clientConnection{},
		draining:       map[string]*clientConnection{},
		resolver:       resolver,
		cancelResolver: func() {},
	}

	// Compute the initial set of endpoints.
	endpoints, version, err := resolver.Resolve(ctx, nil)
	if err != nil {
		return nil, err
	}
	if resolver.IsConstant() && len(endpoints) == 0 {
		// If a constant resolver returns no endpoints, we can return an error
		// immediately. If the resolver is non-constant, we can't. The resolver
		// may return some endpoints in the future.
		return nil, fmt.Errorf("%w: no endpoints available", Unreachable)
	}
	if !resolver.IsConstant() && version == nil {
		return nil, errors.New("non-constant resolver returned a nil version")
	}
	if err := conn.updateEndpoints(endpoints); err != nil {
		return nil, err
	}

	// If the resolver is non-constant, then we start a goroutine to watch for
	// updates to the set of endpoints. If the resolver is constant, then we
	// don't need to do this because the endpoints never change.
	if !resolver.IsConstant() {
		ctx, cancel := context.WithCancel(ctx)
		conn.cancelResolver = cancel
		conn.resolverDone.Add(1)
		go conn.watchResolver(ctx, version)
	}

	return &conn, nil
}

// Close closes a connection.
func (rc *reconnectingConnection) Close() {
	closeWithLock := func() {
		rc.mu.Lock()
		defer rc.mu.Unlock()
		if rc.closed {
			return
		}
		rc.closed = true
		for _, conn := range rc.connections {
			conn.endCalls(fmt.Errorf("%w: %s", CommunicationError, "connection closed"))
		}
		for _, conn := range rc.draining {
			conn.endCalls(fmt.Errorf("%w: %s", CommunicationError, "connection closed"))
		}
	}
	closeWithLock()

	// Cancel the watchResolver goroutine and wait for it to terminate. If the
	// watchResolver has already been terminated, then this code is a no-op.
	// Note that if we hold the lock while waiting for watchResolver to
	// terminate, we may deadlock.
	rc.cancelResolver()
	rc.resolverDone.Wait()
}

// Call makes an RPC over connection c.
func (rc *reconnectingConnection) Call(ctx context.Context, h MethodKey, arg []byte, opts CallOptions) ([]byte, error) {
	var hdr [msgHeaderSize]byte
	copy(hdr[0:], h[:])
	deadline, haveDeadline := ctx.Deadline()
	if haveDeadline {
		// Send the deadline in the header. We use the relative time instead
		// of absolute in case there is significant clock skew. This does mean
		// that we will not count transmission delay against the deadline.
		micros := time.Until(deadline).Microseconds()
		if micros <= 0 {
			// Fail immediately without attempting to send a zero or negative
			// deadline to the server which will be misinterpreted.
			<-ctx.Done()
			return nil, ctx.Err()
		}
		binary.LittleEndian.PutUint64(hdr[16:], uint64(micros))
	}

	// Send trace information in the header.
	writeTraceContext(ctx, hdr[24:])

	rpc := &call{}
	rpc.doneSignal = make(chan struct{})

	// TODO: Arrange to obey deadline in any reconnection done inside startCall.
	//
	// TODO(mwhittaker): Right now, every RPC call is tried on a single server
	// connection. If the call fails, it is not retried. If a call fails on a
	// connection, we may want to try it again on a different connection. We
	// may also want to detect that certain connections are bad and avoid them
	// outright.
	conn, err := rc.startCall(ctx, rpc, opts)
	if err != nil {
		return nil, err
	}

	if err := writeMessage(conn.c, &conn.wlock, requestMessage, rpc.id, hdr[:], arg, rc.opts.WriteFlattenLimit); err != nil {
		conn.shutdown("client send request", err)
		conn.endCall(rpc)
		return nil, fmt.Errorf("%w: %s", CommunicationError, err)
	}

	if rc.opts.OptimisticSpinDuration > 0 {
		// Optimistically spin, waiting for the results.
		for start := time.Now(); time.Since(start) < rc.opts.OptimisticSpinDuration; {
			if atomic.LoadUint32(&rpc.done) > 0 {
				return rpc.response, rpc.err
			}
		}
	}

	if cdone := ctx.Done(); cdone != nil {
		select {
		case <-rpc.doneSignal:
			// Regular return
		case <-cdone:
			// Canceled or deadline expired.
			conn.endCall(rpc)

			if !haveDeadline || time.Now().Before(deadline) {
				// Early cancellation. Tell server about it.
				if err := writeMessage(conn.c, &conn.wlock, cancelMessage, rpc.id, nil, nil, rc.opts.WriteFlattenLimit); err != nil {
					conn.shutdown("client send cancel", err)
				}
			}

			return nil, ctx.Err()
		}
	} else {
		<-rpc.doneSignal
	}
	return rpc.response, rpc.err
}

// watchResolver watches for updates to the set of endpoints. When a new set of
// updates is available, watchResolver passes it to updateEndpoints.
// REQUIRES: version != nil.
// REQUIRES: rc.mu is not held.
func (rc *reconnectingConnection) watchResolver(ctx context.Context, version *Version) {
	defer rc.resolverDone.Done()

	for r := retry.Begin(); r.Continue(ctx); {
		endpoints, newVersion, err := rc.resolver.Resolve(ctx, version)
		if err != nil {
			logError(rc.opts.Logger, "watchResolver", err)
			continue
		}
		if newVersion == nil {
			logError(rc.opts.Logger, "watchResolver", errors.New("non-constant resolver returned a nil version"))
			continue
		}
		if *version == *newVersion {
			// Resolver wishes to be called again after an appropriate delay.
			continue
		}
		if err := rc.updateEndpoints(endpoints); err != nil {
			logError(rc.opts.Logger, "watchResolver", err)
		}
		version = newVersion
		r.Reset()
	}
}

// updateEndpoints updates the set of endpoints. Existing connections are
// retained, and stale connections are closed.
// REQUIRES: rc.mu is not held.
func (rc *reconnectingConnection) updateEndpoints(endpoints []Endpoint) error {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.closed {
		return fmt.Errorf("updateEndpoints on closed Connection")
	}

	// Remove fully drained connections since they have been closed already and
	// cannot be reused.
	rc.removeDrainedConnections()

	// Retain existing connections.
	connections := make(map[string]*clientConnection, len(endpoints))
	for _, endpoint := range endpoints {
		addr := endpoint.Address()
		if conn, ok := rc.connections[addr]; ok {
			connections[addr] = conn
			delete(rc.connections, addr)
		} else if conn, ok := rc.draining[addr]; ok {
			conn.draining = false
			connections[addr] = conn
			delete(rc.draining, addr)
		} else {
			// If we don't have an existing connection, it will be created
			// on-demand when Call is invoked. We don't have to insert anything
			// into rc.connections.
		}
	}

	// Update our state.
	rc.endpoints = endpoints
	for addr, conn := range rc.connections {
		conn.draining = true
		rc.draining[addr] = conn
	}
	rc.connections = connections
	rc.opts.Balancer.Update(endpoints)

	// Close draining connections that don't have any pending requests. If a
	// draining connection does have pending requests, then the connection will
	// close itself when it finishes processing all of its requests.
	rc.removeDrainedConnections()

	// TODO(mwhittaker): Close draining connections after a delay?

	return nil
}

// removeDrainedConnections closes and removes any fully drained connections
// from rc.draining.
//
// REQUIRES: rc.mu is held.
func (rc *reconnectingConnection) removeDrainedConnections() {
	for addr, conn := range rc.draining {
		conn.endIfDrained()
		if conn.ended {
			delete(rc.draining, addr)
		}
	}
}

// startCall registers a new in-progress call.
// REQUIRES: rc.mu is not held.
func (rc *reconnectingConnection) startCall(ctx context.Context, rpc *call, opts CallOptions) (*clientConnection, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.closed {
		return nil, fmt.Errorf("Call on closed Connection")
	}

	if len(rc.endpoints) == 0 {
		return nil, fmt.Errorf("%w: no endpoints available", Unreachable)
	}

	// Note that it is important to hold rc.mu when calling Pick(), and it's
	// important that we index into rc.connections with addr while still
	// holding rc.mu. Otherwise, a Pick() call could operate on a stale set of
	// endpoints and return an endpoint that does not exist in rc.connections.
	var balancer = rc.opts.Balancer
	if opts.Balancer != nil {
		balancer = opts.Balancer
		balancer.Update(rc.endpoints)
	}

	// TODO(mwhittaker): Think about the other places where we can perform
	// automatic retries. We need to be careful about non-idempotent
	// operations.
	var connectErr error
	for i := 0; i < maxReconnectTries; i++ {
		endpoint, err := balancer.Pick(opts)
		if err != nil {
			return nil, err
		}
		addr := endpoint.Address()

		if conn, ok := rc.connections[addr]; !ok || conn.ended {
			c, err := rc.reconnect(ctx, endpoint)
			if err != nil {
				connectErr = err
				continue
			}
			rc.connections[addr] = c
		}

		c := rc.connections[addr]
		c.lastID++
		rpc.id = c.lastID
		c.calls[rpc.id] = rpc
		return c, nil
	}
	return nil, connectErr
}

// reconnect establishes (or re-establishes) the network connection to the server.
// REQUIRES: rc.mu is held.
func (rc *reconnectingConnection) reconnect(ctx context.Context, endpoint Endpoint) (*clientConnection, error) {
	nc, err := endpoint.Dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", CommunicationError, err)
	}
	conn := &clientConnection{
		logger:   rc.opts.Logger,
		endpoint: endpoint,
		c:        nc,
		cbuf:     bufio.NewReader(nc),
		mu:       &rc.mu,
		version:  initialVersion, // Updated when we hear from server
		calls:    map[uint64]*call{},
		lastID:   0,
	}
	if err := writeVersion(conn.c, &conn.wlock); err != nil {
		return nil, fmt.Errorf("%w: client send version: %s", CommunicationError, err)
	}
	go conn.readResponses()
	return conn, nil
}

func (c *clientConnection) endCall(rpc *call) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.calls, rpc.id)
	c.endIfDrained()
}

func (c *clientConnection) findAndEndCall(id uint64) *call {
	c.mu.Lock()
	defer c.mu.Unlock()
	rpc := c.calls[id]
	if rpc != nil {
		delete(c.calls, id)
		c.endIfDrained()
	}
	return rpc
}

// endIfDrained closes c if it is a fully drained connection.
//
// REQUIRES: c.mu is held.
func (c *clientConnection) endIfDrained() {
	// Note that endIfDrained closes c, but it doesn't remove c from
	// reconnectingConnection.draining. rc.updateEndpoints will remove drained
	// connections from rc.draining. This approach leaves some drained
	// connections around, but it simplifies the code. Specifically, a
	// reconnectingConnection may modify a child clientConnection, but a
	// clientConnection never modifies its parent reconnectingConnection.
	if c.draining && len(c.calls) == 0 {
		c.endCalls(fmt.Errorf("connection drained"))
	}
}

// shutdown processes an error detected while operating on a connection.
// It closes the network connection and cancels all requests in progress on the connection.
// REQUIRES: c.mu is not held.
func (c *clientConnection) shutdown(details string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.loggedShutdown {
		c.loggedShutdown = true
		logError(c.logger, "shutdown: "+details, err)
	}

	// Cancel all in-progress calls.
	c.endCalls(fmt.Errorf("%w: %s: %s", CommunicationError, details, err))
}

// endCalls closes the network connection and ends any in-progress calls.
// REQUIRES: c.mu is held.
func (c *clientConnection) endCalls(err error) {
	c.c.Close()
	c.ended = true
	for id, active := range c.calls {
		active.err = err
		atomic.StoreUint32(&active.done, 1)
		close(active.doneSignal)
		delete(c.calls, id)
	}
}

// readResponses runs on the client side reading messages sent over a connection by the server.
func (c *clientConnection) readResponses() {
	for {
		mt, id, msg, err := readMessage(c.cbuf)
		if err != nil {
			c.shutdown("client read", err)
			return
		}

		switch mt {
		case versionMessage:
			v, err := getVersion(id, msg)
			if err != nil {
				c.shutdown("client read", err)
				return
			}
			c.mu.Lock()
			c.version = v
			c.mu.Unlock()
		case responseMessage, responseError:
			rpc := c.findAndEndCall(id)
			if rpc == nil {
				continue // May have been canceled
			}
			if mt == responseError {
				if err, ok := decodeError(msg); ok {
					rpc.err = err
				} else {
					rpc.err = fmt.Errorf("%w: could not decode error", CommunicationError)
				}
			} else {
				rpc.response = msg
			}
			atomic.StoreUint32(&rpc.done, 1)
			close(rpc.doneSignal)
		default:
			c.shutdown("client read", fmt.Errorf("invalid response %d", mt))
			return
		}
	}
}

// readRequests runs on the server side reading messages sent over a connection by the client.
func (c *serverConnection) readRequests(ctx context.Context, hmap *HandlerMap, onDone func()) {
	for ctx.Err() == nil {
		mt, id, msg, err := readMessage(c.cbuf)
		if err != nil {
			c.shutdown("server read", err)
			onDone()
			return
		}

		switch mt {
		case versionMessage:
			v, err := getVersion(id, msg)
			if err != nil {
				c.shutdown("server read version", err)
				onDone()
				return
			}
			c.mu.Lock()
			c.version = v
			c.mu.Unlock()

			// Respond with my version.
			if err := writeVersion(c.c, &c.wlock); err != nil {
				c.shutdown("server send version", err)
				onDone()
				return
			}
		case requestMessage:
			if c.opts.InlineHandlerDuration > 0 {
				// Run the handler inline. If it doesn't return in the specified
				// time period, launch another goroutine to read incoming requests.
				t := time.AfterFunc(c.opts.InlineHandlerDuration, func() {
					c.readRequests(ctx, hmap, onDone)
				})
				c.runHandler(hmap, id, msg)
				if !t.Stop() {
					// Another goroutine is reading incoming requests: bail out.
					return
				}
			} else {
				// Run the handler in a separate goroutine.
				go c.runHandler(hmap, id, msg)
			}
		case cancelMessage:
			c.endRequest(id)
		default:
			c.shutdown("server read", fmt.Errorf("invalid request type %d", mt))
			onDone()
			return
		}
	}
	c.c.Close()
	onDone()
}

// runHandler runs an application specified RPC handler at the server side.
// The result (or error) from the handler is sent back to the client over c.
func (c *serverConnection) runHandler(hmap *HandlerMap, id uint64, msg []byte) {
	// Extract request header from front of payload.
	if len(msg) < msgHeaderSize {
		c.shutdown("server handler", fmt.Errorf("missing request header"))
		return
	}

	// Extract handler key.
	var hkey MethodKey
	copy(hkey[:], msg)

	// Extract the method name
	methodName := hmap.names[hkey]
	if methodName == "" {
		methodName = "handler"
	} else {
		methodName = logging.ShortenComponent(methodName)
	}

	// Extract trace context and create a new child span to trace the method
	// call on the server.
	ctx := context.Background()
	span := trace.SpanFromContext(ctx) // noop span
	if sc := readTraceContext(msg[24:]); sc.IsValid() {
		ctx, span = c.opts.Tracer.Start(trace.ContextWithSpanContext(ctx, sc), methodName, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
	}

	// Add deadline information from the header to the context.
	micros := binary.LittleEndian.Uint64(msg[16:])
	var cancelFunc func()
	if micros != 0 {
		deadline := time.Now().Add(time.Microsecond * time.Duration(micros))
		ctx, cancelFunc = context.WithDeadline(ctx, deadline)
	} else {
		ctx, cancelFunc = context.WithCancel(ctx)
	}
	defer func() {
		if cancelFunc != nil {
			cancelFunc()
		}
	}()

	// Call the handler passing it the payload.
	payload := msg[msgHeaderSize:]
	var err error
	var result []byte
	fn, ok := hmap.handlers[hkey]
	if !ok {
		err = fmt.Errorf("internal error: unknown function")
	} else {
		if err := c.startRequest(id, cancelFunc); err != nil {
			logError(c.opts.Logger, "handle "+hmap.names[hkey], err)
			return
		}
		cancelFunc = nil // endRequest() or cancellation will deal with it
		defer c.endRequest(id)
		result, err = fn(ctx, payload)
	}

	mt := responseMessage
	if err != nil {
		mt = responseError
		result = encodeError(err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	if err := writeMessage(c.c, &c.wlock, mt, id, nil, result, c.opts.WriteFlattenLimit); err != nil {
		c.shutdown("server write "+hmap.names[hkey], err)
	}
}

func (c *serverConnection) startRequest(id uint64, cancelFunc func()) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("startRequest: %w", net.ErrClosed)
	}
	c.cancelFuncs[id] = cancelFunc
	return nil
}

func (c *serverConnection) endRequest(id uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cancelFunc, ok := c.cancelFuncs[id]; ok {
		delete(c.cancelFuncs, id)
		cancelFunc()
	}
}

// shutdown processes an error detected while operating on a connection.
// It cancels all requests in progress on the connection.
func (c *serverConnection) shutdown(details string, err error) {
	c.c.Close()

	// Cancel all in-progress server-side request handlers.
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		logError(c.opts.Logger, "shutdown: "+details, err)
	}
	for id, cf := range c.cancelFuncs {
		cf()
		delete(c.cancelFuncs, id)
	}
}

func logError(logger *slog.Logger, details string, err error) {
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.ErrClosedPipe) {
		logger.Info(details, "err", err)
	} else {
		logger.Error(details, "err", err)
	}
}
