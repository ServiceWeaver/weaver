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

package protomsg

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"github.com/ServiceWeaver/weaver/runtime/protos"
)

func TestReadWrite(t *testing.T) {
	seq := []proto.Message{
		&protos.Deployment{Id: "dep1"},
		&protos.Deployment{Id: "dep2"},
		&protos.Deployment{Id: "dep3"},
	}

	buf := &bytes.Buffer{}
	for _, msg := range seq {
		if err := Write(buf, msg); err != nil {
			t.Fatal(err)
		}
	}

	var read []proto.Message
	for buf.Len() != 0 {
		msg := &protos.Deployment{}
		if err := Read(buf, msg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		read = append(read, msg)
	}

	if diff := cmp.Diff(seq, read, protocmp.Transform()); diff != "" {
		t.Fatalf("protomsg.Read returned wrong results (-want, +got):\n%s", diff)
	}
}
