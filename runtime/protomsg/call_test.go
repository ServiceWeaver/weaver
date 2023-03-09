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
	"testing"

	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

var (
	msg1 = &protos.ReplicaToRegister{
		App:            "foo",
		GroupReplicaId: "1234",
		DeploymentId:   "5678",
		Group:          "bar",
		Address:        "qux",
		Pid:            42,
	}

	msg2 = &protos.ExportListenerRequest{
		Listener: &protos.Listener{
			Name: "lis",
			Addr: "lis_addr",
		},
	}

	msg3 = &protos.MetricUpdate{
		Defs: []*protos.MetricDef{
			{
				Id:   1,
				Name: "def1",
				Labels: map[string]string{
					"serviceweaver/component": "comp1",
					"l1":                      "v1",
					"l2":                      "v2",
					"l3":                      "v3",
				},
				Bounds: []float64{1.0, 4.0, 10.0},
			},
		},
		Values: []*protos.MetricValue{
			{Id: 1, Value: 1},
			{Id: 2, Value: 2},
			{Id: 3, Value: 3},
			{Id: 4, Value: 4},
		},
	}
)

// TestWire tests that ToWire() followed by FromWire() is an identity function,
// i.e., it results in the original set of messages.
func TestWire(t *testing.T) {
	for _, msgs := range [][]proto.Message{
		{msg1},
		{msg2},
		{msg3},
		{msg1, msg2},
		{msg1, msg3},
		{msg2, msg3},
		{msg1, msg2, msg3},
	} {
		data, err := toWire(msgs...)
		if err != nil {
			t.Error(err)
		}
		orig := make([]proto.Message, len(msgs))
		for i, msg := range msgs {
			orig[i] = proto.Clone(msg)
			proto.Reset(msg)
		}
		if err := fromWire(data, msgs...); err != nil {
			t.Error(err)
		}
		if diff := cmp.Diff(orig, msgs, protocmp.Transform()); diff != "" {
			t.Fatalf("wire diff (-want, +got):\n%s", diff)
		}
	}
}
