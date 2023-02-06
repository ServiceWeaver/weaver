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

import "testing"

func TestHash(t *testing.T) {
	// Test that the Hash function is deterministic and handles all supported types.
	hashers := make([]Hasher, 2)
	for i := range hashers {
		hashers[i].WriteString("string")
		hashers[i].WriteFloat32(1.0)
		hashers[i].WriteFloat64(2.0)
		hashers[i].WriteInt(3)
		hashers[i].WriteInt8(4)
		hashers[i].WriteInt16(5)
		hashers[i].WriteInt32(6)
		hashers[i].WriteInt64(7)
		hashers[i].WriteUint(8)
		hashers[i].WriteUint8(9)
		hashers[i].WriteUint16(10)
		hashers[i].WriteUint32(11)
		hashers[i].WriteUint64(12)
	}
	a, b := hashers[0].Sum64(), hashers[1].Sum64()
	if a != b {
		t.Errorf("non-deterministic hash values %016x, %016x", a, b)
	}

	// Also verify that the hash value is stable across processes.
	const expected uint64 = 0xd04e633fa4f670db
	if a != expected {
		t.Errorf("unstable hash value %016x (expecting %016x)", a, expected)
	}
}
