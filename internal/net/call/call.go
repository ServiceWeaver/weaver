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
// For each newly discovered server, the client starts a manage() goroutine
// that connects to server, and then reads messages from the connection. If the
// network connection breaks, manage() reconnects (after a retry delay).
//
// When the client wants to send an RPC, it selects one of its server
// connections to use, creates a call object, assigns it a new request-id, and
// registers the object in a map in the connection. It then sends a request
// message over the connection and waits for the call object to be marked as
// done.
//
// When the response arrives, it is picked up by readAndProcessMessage().
// readAndProcessMessage() finds the call object corresponding to the
// request-id in the response, and marks the call object as done which
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
// connection that hasn't closed itself, the draining connection is turned
// back into a normal connection.
import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/retry"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	// Size of the header included in each message.
	msgHeaderSize = 16 + 8 + traceHeaderLen // handler_key + deadline + trace_context
)

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
	mu     sync.Mutex
	conns  map[string]*clientConnection
	closed bool

	resolver       Resolver
	cancelResolver func()         // cancels the watchResolver goroutine
	resolverDone   sync.WaitGroup // used to wait for watchResolver to finish
}

// connState is the state of a clientConnection (connection to a particular
// server replica). missing is a special state used for unknown servers. A
// typical sequence of transitions is:
//
//	missing -> disconnected -> checking -> idle <-> active -> draining -> missing
//
// The events that can cause state transition are:
//
// - register: server has shown up in resolver results
// - unregister: server has dropped from resolver results
// - connected: a connection has been successfully made
// - checked: connection has been successfully checked
// - callstart: call starts on connection
// - lastdone: last active call on connection has ended
// - fail: some protocol error is detected on the connection
// - close: reconnectingConnection is being closed
//
// Each event has a corresponding clientConnection method below. See
// those methods for the corresponding state transitions.
type connState int8

const (
	missing      connState = iota
	disconnected           // cannot be used for calls
	checking               // checking new network connection
	idle                   // can be used for calls, no calls in-flight
	active                 // can be used for calls, some calls in-flight
	draining               // some calls in-flight, no new calls should be added
)

var connStateNames = []string{
	"missing",
	"disconnected",
	"checking",
	"idle",
	"active",
	"draining",
}

func (s connState) String() string { return connStateNames[s] }

// clientConnection manages one network connection on the client-side.
type clientConnection struct {
	// Immutable after construction.
	rc       *reconnectingConnection // owner
	canceler func()                  // Used to cancel goroutine handling connection
	logger   *slog.Logger
	endpoint Endpoint

	wlock sync.Mutex // Guards writes to c

	// Guarded by rc.mu
	state          connState        // current connection state
	loggedShutdown bool             // Have we logged a shutdown error?
	inBalancer     bool             // Is c registered with the balancer?
	c              net.Conn         // Active network connection, or nil
	cbuf           *bufio.Reader    // Buffered reader wrapped around c
	version        version          // Version number to use for connection
	calls          map[uint64]*call // In-progress calls
	lastID         uint64           // Last assigned request ID for a call
}

var _ ReplicaConnection = &clientConnection{}

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
	l = &onceCloseListener{Listener: l, closer: sync.OnceValue(l.Close)}

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
type onceCloseListener struct {
	Listener
	closer func() error // Must be result of sync.OnceValue
}

func (oc *onceCloseListener) Close() error {
	return oc.closer()
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
		conns:          map[string]*clientConnection{},
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
	if err := conn.updateEndpoints(ctx, endpoints); err != nil {
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
		for _, c := range rc.conns {
			c.close()
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

// Call makes an RPC over connection c, retrying it on network errors if retries are allowed.
func (rc *reconnectingConnection) Call(ctx context.Context, h MethodKey, arg []byte, opts CallOptions) ([]byte, error) {
	if !opts.Retry {
		return rc.callOnce(ctx, h, arg, opts)
	}
	for r := retry.Begin(); r.Continue(ctx); {
		response, err := rc.callOnce(ctx, h, arg, opts)
		if errors.Is(err, Unreachable) || errors.Is(err, CommunicationError) {
			continue
		}
		return response, err
	}
	return nil, ctx.Err()
}

func (rc *reconnectingConnection) callOnce(ctx context.Context, h MethodKey, arg []byte, opts CallOptions) ([]byte, error) {
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
	conn, nc, err := rc.startCall(ctx, rpc, opts)
	if err != nil {
		return nil, err
	}
	if err := writeMessage(nc, &conn.wlock, requestMessage, rpc.id, hdr[:], arg, rc.opts.WriteFlattenLimit); err != nil {
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
				if err := writeMessage(nc, &conn.wlock, cancelMessage, rpc.id, nil, nil, rc.opts.WriteFlattenLimit); err != nil {
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
		if err := rc.updateEndpoints(ctx, endpoints); err != nil {
			logError(rc.opts.Logger, "watchResolver", err)
		}
		version = newVersion
		r.Reset()
	}
}

// updateEndpoints updates the set of endpoints. Existing connections are
// retained, and stale connections are closed.
// REQUIRES: rc.mu is not held.
func (rc *reconnectingConnection) updateEndpoints(ctx context.Context, endpoints []Endpoint) error {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.closed {
		return fmt.Errorf("updateEndpoints on closed Connection")
	}

	// Make new endpoints.
	keep := make(map[string]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		addr := endpoint.Address()
		keep[addr] = struct{}{}
		if _, ok := rc.conns[addr]; !ok {
			// New endpoint, create connection and manage it.
			ctx, cancel := context.WithCancel(ctx)
			c := &clientConnection{
				rc:       rc,
				canceler: cancel,
				logger:   rc.opts.Logger,
				endpoint: endpoint,
				calls:    map[uint64]*call{},
				lastID:   0,
			}
			rc.conns[addr] = c
			c.register()
			go c.manage(ctx)
		}
	}

	// Drop old endpoints.
	for addr, c := range rc.conns {
		if _, ok := keep[addr]; ok {
			// Still live, so keep it.
			continue
		}
		c.unregister()
	}

	return nil
}

// startCall registers a new in-progress call.
// REQUIRES: rc.mu is not held.
func (rc *reconnectingConnection) startCall(ctx context.Context, rpc *call, opts CallOptions) (*clientConnection, net.Conn, error) {
	for r := retry.Begin(); r.Continue(ctx); {
		rc.mu.Lock()
		if rc.closed {
			rc.mu.Unlock()
			return nil, nil, fmt.Errorf("Call on closed Connection")
		}

		replica, ok := rc.opts.Balancer.Pick(opts)
		if !ok {
			rc.mu.Unlock()
			continue
		}

		c, ok := replica.(*clientConnection)
		if !ok {
			rc.mu.Unlock()
			return nil, nil, fmt.Errorf("internal error: wrong connection type %#v returned by load balancer", replica)
		}

		c.lastID++
		rpc.id = c.lastID
		c.calls[rpc.id] = rpc
		c.callstart()
		nc := c.c
		rc.mu.Unlock()

		return c, nc, nil
	}

	return nil, nil, ctx.Err()
}

func (c *clientConnection) Address() string {
	return c.endpoint.Address()
}

// State transition actions: all of these are called with rc.mu held.

func (c *clientConnection) register() {
	switch c.state {
	case missing:
		c.setState(disconnected)
	case draining:
		// We were attempting to get rid of the old connection, but it
		// seems like the server-side problem was transient, so we
		// resurrect the draining connection into a non-draining state.
		//
		// New state is active instead of idle since state==draining
		// implies there is at least one call in-flight.
		c.setState(active)
	}
}

func (c *clientConnection) unregister() {
	switch c.state {
	case disconnected, checking, idle:
		c.setState(missing)
	case active:
		c.setState(draining)
	}
}

func (c *clientConnection) connected() {
	switch c.state {
	case disconnected:
		c.setState(checking)
	}
}

func (c *clientConnection) checked() {
	switch c.state {
	case checking:
		c.setState(idle)
	}
}

func (c *clientConnection) callstart() {
	switch c.state {
	case idle:
		c.setState(active)
	}
}

func (c *clientConnection) lastdone() {
	switch c.state {
	case active:
		c.setState(idle)
	case draining:
		c.setState(missing)
	}
}

func (c *clientConnection) fail(details string, err error) {
	if !c.loggedShutdown {
		c.loggedShutdown = true
		logError(c.logger, details, err)
	}

	// endCalls here so we can supply good errors.
	c.endCalls(fmt.Errorf("%w: %s: %s", CommunicationError, details, err))

	switch c.state {
	case checking, idle, active:
		c.setState(disconnected)
	case draining:
		c.setState(missing)
	}
}

func (c *clientConnection) close() {
	// endCalls here so we can supply good errors.
	c.endCalls(fmt.Errorf("%w: connection closed", CommunicationError))

	c.setState(missing)
}

// checkInvariants verifies clientConnection invariants.
func (c *clientConnection) checkInvariants() {
	s := c.state

	// connection in reconnectingConnection.conns iff state not in {missing}
	if _, ok := c.rc.conns[c.endpoint.Address()]; ok != (s != missing) {
		panic(fmt.Sprintf("%v connection: wrong connection table presence %v", s, ok))
	}

	// has net.Conn iff state in {checking, idle, active, draining}
	if (c.c != nil) != (s == checking || s == idle || s == active || s == draining) {
		panic(fmt.Sprintf("%v connection: wrong net.Conn %v", s, c.c))
	}

	// connection is in the balancer iff state in {idle, active}
	if c.inBalancer != (s == idle || s == active) {
		panic(fmt.Sprintf("%v connection: wrong balancer presence %v", s, c.inBalancer))
	}

	// len(calls) > 0 iff state in {active, draining}
	if (len(c.calls) != 0) != (s == active || s == draining) {
		panic(fmt.Sprintf("%v connection: wrong number of calls %d", s, len(c.calls)))
	}
}

// setState transitions to state s and updates any related state.
func (c *clientConnection) setState(s connState) {
	// idle<-> active transitions may happen a lot, so short-circuit them
	// by avoiding logging and full invariant maintenance.
	if c.state == active && s == idle {
		c.state = idle
		if len(c.calls) != 0 {
			panic(fmt.Sprintf("%v connection: wrong number of calls %d", s, len(c.calls)))
		}
		return
	} else if c.state == idle && s == active {
		c.state = active
		if len(c.calls) == 0 {
			panic(fmt.Sprintf("%v connection: wrong number of calls %d", s, len(c.calls)))
		}
		return
	}

	c.logger.Info("connection", "addr", c.endpoint.Address(), "from", c.state, "to", s)
	c.state = s

	// Fix membership in rc.conns.
	if s == missing {
		delete(c.rc.conns, c.endpoint.Address())
		if c.canceler != nil {
			c.canceler() // Forces retry loop to end early
			c.canceler = nil
		}
	} // else: caller is responsible for adding c to rc.conns

	// Fix net.Conn presence.
	if s == missing || s == disconnected {
		if c.c != nil {
			c.c.Close()
			c.c = nil
			c.cbuf = nil
		}
	} // else: caller is responsible for setting c.c and c.cbuf

	// Fix balancer membership.
	if s == idle || s == active {
		if !c.inBalancer {
			c.rc.opts.Balancer.Add(c)
			c.inBalancer = true
		}
	} else {
		if c.inBalancer {
			c.rc.opts.Balancer.Remove(c)
			c.inBalancer = false
		}
	}

	// Fix in-flight calls.
	if s == active || s == draining {
		// Keep calls live
	} else {
		c.endCalls(fmt.Errorf("%w: %v", CommunicationError, s))
	}

	c.checkInvariants()
}

func (c *clientConnection) endCall(rpc *call) {
	c.rc.mu.Lock()
	defer c.rc.mu.Unlock()
	delete(c.calls, rpc.id)
	if len(c.calls) == 0 {
		c.lastdone()
	}
}

func (c *clientConnection) findAndEndCall(id uint64) *call {
	c.rc.mu.Lock()
	defer c.rc.mu.Unlock()
	rpc := c.calls[id]
	if rpc != nil {
		delete(c.calls, id)
		if len(c.calls) == 0 {
			c.lastdone()
		}
	}
	return rpc
}

// shutdown processes an error detected while operating on a connection.
// It closes the network connection and cancels all requests in progress on the connection.
// REQUIRES: c.mu is not held.
func (c *clientConnection) shutdown(details string, err error) {
	c.rc.mu.Lock()
	defer c.rc.mu.Unlock()
	c.fail(details, err)
}

// endCalls closes the network connection and ends any in-progress calls.
// REQUIRES: c.mu is held.
func (c *clientConnection) endCalls(err error) {
	for id, active := range c.calls {
		active.err = err
		atomic.StoreUint32(&active.done, 1)
		close(active.doneSignal)
		delete(c.calls, id)
	}
}

// manage handles a live clientConnection until it becomes missing.
func (c *clientConnection) manage(ctx context.Context) {
	for r := retry.Begin(); r.Continue(ctx); {
		progress := c.connectOnce(ctx)
		if progress {
			r.Reset()
		}
	}
}

// connectOnce dials once to the endpoint and manages the resulting connection.
// It returns true if some communication happened successfully over the connection.
func (c *clientConnection) connectOnce(ctx context.Context) bool {
	// Dial the connection.
	nc, err := c.endpoint.Dial(ctx)
	if err != nil {
		logError(c.logger, "dial", err)
		return false
	}
	defer nc.Close()

	c.rc.mu.Lock()
	defer c.rc.mu.Unlock() // Also temporarily unlocked below
	c.c = nc
	c.cbuf = bufio.NewReader(nc)
	c.loggedShutdown = false
	c.connected()

	// Handshake to get the peer version and verify that it is live.
	if err := c.exchangeVersions(); err != nil {
		c.fail("handshake", err)
		return false
	}
	c.checked()

	for c.state == idle || c.state == active || c.state == draining {
		if err := c.readAndProcessMessage(); err != nil {
			c.fail("client read", err)
		}
	}
	return true
}

// exchangeVersions sends client version to server and waits for the server version.
func (c *clientConnection) exchangeVersions() error {
	nc, buf := c.c, c.cbuf

	// Do not hold mutex while reading from the network.
	c.rc.mu.Unlock()
	defer c.rc.mu.Lock()

	if err := writeVersion(nc, &c.wlock); err != nil {
		return err
	}
	mt, id, msg, err := readMessage(buf)
	if err != nil {
		return err
	}
	if mt != versionMessage {
		return fmt.Errorf("wrong message type %d, expecting %d", mt, versionMessage)
	}
	v, err := getVersion(id, msg)
	if err != nil {
		return err
	}
	c.version = v
	return nil
}

// readAndProcessMessage reads and handles one message sent from the server.
func (c *clientConnection) readAndProcessMessage() error {
	buf := c.cbuf

	// Do not hold mutex while reading from the network.
	c.rc.mu.Unlock()
	defer c.rc.mu.Lock()

	mt, id, msg, err := readMessage(buf)
	if err != nil {
		return err
	}
	switch mt {
	case versionMessage:
		_, err := getVersion(id, msg)
		if err != nil {
			return err
		}
		// Ignore versions sent after initial hand-shake
	case responseMessage, responseError:
		rpc := c.findAndEndCall(id)
		if rpc == nil {
			return nil // May have been canceled
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
		return fmt.Errorf("invalid response %d", mt)
	}
	return nil
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
