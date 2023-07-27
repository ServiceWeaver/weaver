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

package call

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"

	"go.opentelemetry.io/otel/trace"
)

// messageType identifies a type of message sent across the wire.
type messageType uint8

const (
	versionType messageType = iota
	requestType
	responseType
	errorType
	cancelType
	// Other types to add?
	// - chunked request/response messages?
	// - health check
	// - server status info
)

// version holds the protocol version number.
type version uint32

const (
	initialVersion version = iota
)

const currentVersion = initialVersion

// # Message formats
//
// All messages have the following format:
//    id        [8]byte       -- identifier used to track the message
//    type      [1]byte       -- messageType
//    length    [7]byte       -- length of the remainder of the message
//    payload   [length]byte  -- message-type-specific data
//
// The format of payload depends on the message type. See below for details.

// message is the interface implemented by messages.
type message interface {
	isMessage()
}

// versionMessage is the first message exchanged by a client and server. The
// version handshake establishes which version of the protocol the client and
// server use.
type versionMessage struct {
	v version // [4]byte -- version
}

// requestMessage is a request to execute an RPC.
type requestMessage struct {
	id             uint64            // RPC id
	methodKey      MethodKey         // [16]byte -- fingerprint of method name
	deadlineMicros uint64            //  [8]byte -- zero, or deadline in microseconds
	spanContext    trace.SpanContext // [25]byte -- zero, or span context
	args           []byte            //   []byte -- call arguments
}

// responseMessage holds the return values of a successful RPC.
type responseMessage struct {
	id      uint64 // RPC id
	returns []byte // []byte -- call returns
}

// errorMessage holds an error encountered when executing an RPC.
type errorMessage struct {
	id  uint64 // RPC id
	err error  // []byte -- codegen encoded error
}

// cancelMessage cancels the execution of an RPC.
type cancelMessage struct {
	id uint64 // RPC id
}

func (versionMessage) isMessage()  {}
func (requestMessage) isMessage()  {}
func (responseMessage) isMessage() {}
func (errorMessage) isMessage()    {}
func (cancelMessage) isMessage()   {}

// writeMessage formats and sends a message over w. The write is guarded by
// wlock, which must not be locked when passed in.
func writeMessage(w io.Writer, wlock *sync.Mutex, msg message, flattenLimit int) error {
	switch x := msg.(type) {
	case versionMessage:
		var msg [4]byte
		binary.LittleEndian.PutUint32(msg[:], uint32(x.v))
		return write(w, wlock, versionType, 0, nil, msg[:], flattenLimit)

	case requestMessage:
		var hdr [msgHeaderSize]byte
		copy(hdr[0:], x.methodKey[:])
		if x.deadlineMicros > 0 {
			binary.LittleEndian.PutUint64(hdr[16:], x.deadlineMicros)
		}
		writeSpanContext(x.spanContext, hdr[24:])
		return write(w, wlock, requestType, x.id, hdr[:], x.args, flattenLimit)

	case responseMessage:
		return write(w, wlock, responseType, x.id, nil, x.returns, flattenLimit)

	case errorMessage:
		return write(w, wlock, errorType, x.id, nil, encodeError(x.err), flattenLimit)

	case cancelMessage:
		return write(w, wlock, cancelType, x.id, nil, nil, flattenLimit)

	default:
		panic(fmt.Errorf("unexpected message type: %T", msg))
	}
}

// write formats and sends a message over w.
//
// The message payload is formed by concatenating extraHdr and payload.
// (Allowing two arguments to form the payload avoids unnecessary allocation
// and copying when we want to prepend some data to application supplied data).
//
// The write is guarded by wlock, which must not be locked when passed in.
func write(w io.Writer, wlock *sync.Mutex, mt messageType, id uint64, extraHdr []byte, payload []byte, flattenLimit int) error {
	nh, np := len(extraHdr), len(payload)
	size := 16 + nh + np
	if size > flattenLimit {
		return writeChunked(w, wlock, mt, id, extraHdr, payload)
	}
	return writeFlat(w, wlock, mt, id, extraHdr, payload)
}

// writeChunked writes the header, extra header, and the payload into w using
// three different w.Write() calls.
func writeChunked(w io.Writer, wlock *sync.Mutex, mt messageType, id uint64, extraHdr []byte, payload []byte) error {
	// We use an iovec with up to three entries.
	var vec [3][]byte

	nh, np := len(extraHdr), len(payload)
	var hdr [16]byte
	binary.LittleEndian.PutUint64(hdr[0:], id)
	binary.LittleEndian.PutUint64(hdr[8:], uint64(mt)|(uint64(nh+np)<<8))

	vec[0] = hdr[:]
	vec[1] = extraHdr
	vec[2] = payload
	buf := net.Buffers(vec[:])

	// buf.WriteTo is not guaranteed to write the entire contents of buf
	// atomically, so we guard the write with a lock to prevent writes from
	// interleaving.
	wlock.Lock()
	defer wlock.Unlock()
	n, err := buf.WriteTo(w)
	if err == nil && n != 16+int64(nh)+int64(np) {
		err = fmt.Errorf("partial write")
	}
	return err
}

// writeFlat concatenates the header, extra header, and the payload into
// a single flat byte slice, and writes it into w using a single w.Write() call.
func writeFlat(w io.Writer, wlock *sync.Mutex, mt messageType, id uint64, extraHdr []byte, payload []byte) error {
	nh, np := len(extraHdr), len(payload)
	data := make([]byte, 16+nh+np)
	binary.LittleEndian.PutUint64(data[0:], id)
	val := uint64(mt) | (uint64(nh+np) << 8)
	binary.LittleEndian.PutUint64(data[8:], val)
	copy(data[16:], extraHdr)
	copy(data[16+nh:], payload)

	// Write while holding the lock, since we don't know if the underlying
	// io.Write is atomic.
	// TODO(mwhittaker): For those io.Writers that are atomic, we can avoid
	// locking in some cases.
	wlock.Lock()
	defer wlock.Unlock()
	n, err := w.Write(data)
	if err == nil && n != len(data) {
		err = fmt.Errorf("partial write")
	}
	return err
}

// read reads, parses, and returns the next message from r.
func read(r io.Reader) (messageType, uint64, []byte, error) {
	// Read the header.
	const headerSize = 16
	var hdr [headerSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, 0, nil, err
	}

	// Extract header contents (see writeMessage for header format).
	id := binary.LittleEndian.Uint64(hdr[0:])
	w2 := binary.LittleEndian.Uint64(hdr[8:])
	mt := messageType(w2 & 0xff)
	dataLen := w2 >> 8
	const maxSize = 100 << 20
	if dataLen > maxSize {
		return 0, 0, nil, fmt.Errorf("overly large message length %d", dataLen)
	}

	// Read the payload.
	msg := make([]byte, int(dataLen))
	if _, err := io.ReadFull(r, msg); err != nil {
		return 0, 0, nil, err
	}
	return mt, id, msg, nil
}

// readMessage reads, parses, and returns the next message from r.
func readMessage(r io.Reader) (message, error) {
	mt, id, payload, err := read(r)
	if err != nil {
		return nil, err
	}

	// Form the message.
	switch mt {
	case versionType:
		if id != 0 {
			return nil, fmt.Errorf("invalid ID %d in handshake", id)
		}
		// Allow messages longer than needed so that future updates can send
		// more info.
		if len(payload) < 4 {
			return nil, fmt.Errorf("bad version message length %d, must be >= 4", len(payload))
		}
		v := version(binary.LittleEndian.Uint32(payload))
		return versionMessage{v: v}, nil

	case requestType:
		if len(payload) < msgHeaderSize {
			return nil, fmt.Errorf("missing request header")
		}

		// Extract method key.
		var methodKey MethodKey
		copy(methodKey[:], payload)

		// Extract deadline.
		deadlineMicros := binary.LittleEndian.Uint64(payload[16:])

		// Extract span context.
		var spanContext trace.SpanContext
		if x := readSpanContext(payload[24:]); x.IsValid() {
			spanContext = x
		}

		return requestMessage{
			id:             id,
			methodKey:      methodKey,
			deadlineMicros: deadlineMicros,
			spanContext:    spanContext,
			args:           payload[msgHeaderSize:],
		}, nil

	case responseType:
		return responseMessage{id: id, returns: payload}, nil

	case errorType:
		if err, ok := decodeError(payload); ok {
			return errorMessage{id: id, err: err}, nil
		}
		err := fmt.Errorf("%w: could not decode error", CommunicationError)
		return errorMessage{id: id, err: err}, nil

	case cancelType:
		return cancelMessage{id: id}, nil

	default:
		panic(fmt.Errorf("unexpected message type: %v", mt))
	}
}
