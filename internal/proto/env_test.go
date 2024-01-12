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

package proto

import (
	"testing"

	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

var (
	msg_1 = &protos.WeaveletArgs{
		App:          "foo",
		DeploymentId: "5678",
		Id:           "id",
	}
	msg_2 = &protos.MetricUpdate{
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

// TestEnv tests that ToEnv() followed by FromEnv() is an identity function,
// i.e., it results in the original protocol message.
func TestEnv(t *testing.T) {
	for _, msg := range []proto.Message{msg_1, msg_2} {
		data, err := ToEnv(msg)
		if err != nil {
			t.Error(err)
		}
		orig := proto.Clone(msg)
		proto.Reset(msg)
		if err := FromEnv(data, msg); err != nil {
			t.Error(err)
		}
		if diff := cmp.Diff(orig, msg, protocmp.Transform()); diff != "" {
			t.Errorf("env diff (-want, +got):\n%s", diff)
		}
	}
}
