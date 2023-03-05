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
)

// messageType identifies a type of message sent across the wire.
type messageType uint8

const (
	versionMessage messageType = iota
	requestMessage
	responseMessage
	responseError
	cancelMessage
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
//	id	[8]byte
//	type	[1]byte		-- messageType
//	length	[7]byte		-- length of the remainder of the message
//	payload	[length]byte	-- message-type-specific data
//
// The format of payload depends on the message typo.
//
// versionMessage: this is the first message sent on a connection by both sides.
//      version  [4]byte
//
//
//  requestMessage:
//	headerKey    [16]byte   -- fingerprint of method name
//	deadline      [8]byte   -- zero, or deadline in microseconds
//  traceContext [25]byte   -- zero, or trace context
//	remainder		        -- call argument serialization
//
// responseMessage:
//	payload holds call result serialization
//
// responseError:
//	payload holds error serialization
//
// cancelMessage:
//	payload is empty

// writeMessage formats and sends a message over w.
//
// The message payload is formed by concatenating extraHdr and payload.
// (Allowing two arguments to form the payload avoids unnecessary allocation
// and copying when we want to prepend some data to application supplied data).
//
// The write is guarded by wlock, which must not be locked when passed in.
func writeMessage(w io.Writer, wlock *sync.Mutex, mt messageType, id uint64, extraHdr []byte, payload []byte, flattenLimit int) error {
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

// readMessage reads, parses, and returns the next message from r.
func readMessage(r io.Reader) (messageType, uint64, []byte, error) {
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

// writeVersion sends my version number to the peer.
func writeVersion(w io.Writer, wlock *sync.Mutex) error {
	var msg [4]byte
	binary.LittleEndian.PutUint32(msg[:], uint32(currentVersion))
	return writeFlat(w, wlock, versionMessage, 0, nil, msg[:])
}

// getVersion extracts the version number sent by the peer and picks the
// appropriate version number to use for communicating with the peer.
func getVersion(id uint64, msg []byte) (version, error) {
	if id != 0 {
		return 0, fmt.Errorf("invalid ID %d in handshake", id)
	}
	// Allow messages longer than needed so that future updates can send more info.
	if len(msg) < 4 {
		return 0, fmt.Errorf("bad version message length %d, must be >= 4", len(msg))
	}
	v := binary.LittleEndian.Uint32(msg)

	// We use the minimum of the peer and my version numbers.
	if v < uint32(currentVersion) {
		return version(v), nil
	}
	return currentVersion, nil
}
