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

package codegen

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
)

// Hasher computes a non-cryptographic hash of the sequence of values
// added to it.
//
// If the same sequence of values is added to two differ Hashers, they
// will produce the same result, even if they are in different processes.
type Hasher struct {
	// TODO: improve performance:
	// - do not accumulate everything; hash as we go
	// - use a non-cryptographically safe hasher
	enc Encoder
}

// Sum64 returns the 64-bit hash of the sequence of values added so far.
// The resulting is in the range [1,2^64-2], i.e., it is never 0 or math.MaxUint64.
func (h *Hasher) Sum64() uint64 {
	bytes := sha256.Sum256(h.enc.Data())
	hash := binary.LittleEndian.Uint64(bytes[:8])
	if hash == 0 {
		// We avoid using a hash of 0 so that a default value of 0 can be
		// interpreted as an invalid or missing hash.
		return 1
	}
	if hash == math.MaxUint64 {
		// We also avoid using a hash of 2^64-1. If we had a range [a, b) that
		// included the maximum hash 2^64-1, what would be the value of b? It
		// would have to be greater than 2^64-1, but there is no such value.
		// Instead, we reserve 2^64-1, allowing ranges to have simple exclusive
		// upper bounds.
		return math.MaxUint64 - 1
	}
	return hash
}

// WriteString adds a string to the hasher.
func (h *Hasher) WriteString(v string) { h.enc.String(v) }

// WriteFloat32 adds a float32 to the hasher.
func (h *Hasher) WriteFloat32(v float32) { h.enc.Float32(v) }

// WriteFloat64 adds a float64 to the hasher.
func (h *Hasher) WriteFloat64(v float64) { h.enc.Float64(v) }

// WriteInt adds a int to the hasher.
func (h *Hasher) WriteInt(v int) { h.enc.Int(v) }

// WriteInt8 adds a int8 to the hasher.
func (h *Hasher) WriteInt8(v int8) { h.enc.Int8(v) }

// WriteInt16 adds a int16 to the hasher.
func (h *Hasher) WriteInt16(v int16) { h.enc.Int16(v) }

// WriteInt32 adds a int32 to the hasher.
func (h *Hasher) WriteInt32(v int32) { h.enc.Int32(v) }

// WriteInt64 adds a int64 to the hasher.
func (h *Hasher) WriteInt64(v int64) { h.enc.Int64(v) }

// WriteUint adds a uint to the hasher.
func (h *Hasher) WriteUint(v uint) { h.enc.Uint(v) }

// WriteUint8 adds a uint8 to the hasher.
func (h *Hasher) WriteUint8(v uint8) { h.enc.Uint8(v) }

// WriteUint16 adds a uint16 to the hasher.
func (h *Hasher) WriteUint16(v uint16) { h.enc.Uint16(v) }

// WriteUint32 adds a uint32 to the hasher.
func (h *Hasher) WriteUint32(v uint32) { h.enc.Uint32(v) }

// WriteUint64 adds a uint64 to the hasher.
func (h *Hasher) WriteUint64(v uint64) { h.enc.Uint64(v) }
