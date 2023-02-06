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

// Package protomsg contains protobuf-related utilities.
package protomsg

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"google.golang.org/protobuf/proto"
)

// maxMsgSize is the largest supported protobuf message.
const maxMsgSize = math.MaxInt32

// Write writes a length prefixed protobuf to dst. Use Read to read it.
func Write(dst io.Writer, msg proto.Message) error {
	enc, err := proto.Marshal(msg)
	if err != nil {
		return nil
	}
	if len(enc) > maxMsgSize {
		return fmt.Errorf("write protobuf: message size %d is too large", len(enc))
	}
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], uint32(len(enc)))
	if _, err := dst.Write(hdr[:]); err != nil {
		return fmt.Errorf("write protobuf length: %w", err)
	}
	if _, err := dst.Write(enc); err != nil {
		return fmt.Errorf("writer protobuf data: %w", err)
	}
	return nil
}

// Read reads a length-prefixed protobuf from src. Messages above maxMsgSize
// are not supported and cause an error to be returned.
func Read(src io.Reader, msg proto.Message) error {
	var hdr [4]byte
	if _, err := io.ReadFull(src, hdr[:]); err != nil {
		return fmt.Errorf("read protobuf length: %w", err)
	}
	n := binary.LittleEndian.Uint32(hdr[:])
	if n > maxMsgSize {
		return fmt.Errorf("read protobuf: message size %d is too large", n)
	}
	data := make([]byte, int(n))
	if _, err := io.ReadFull(src, data); err != nil {
		return fmt.Errorf("read protobuf data %d: %w", n, err)
	}
	return proto.Unmarshal(data, msg)
}
