// Copyright 2023 Google LLC
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

package codegen_test

import (
	"reflect"
	"testing"

	"github.com/ServiceWeaver/weaver/runtime/codegen"
)

func TestGraphEdges(t *testing.T) {
	a2b := codegen.MakeEdgeString("a", "b")
	a2c := codegen.MakeEdgeString("a", "c")
	c2d := codegen.MakeEdgeString("c", "d")
	b2c := codegen.MakeEdgeString("b", "c")
	data := a2b + a2c + c2d + b2c
	t.Log(data)

	got := codegen.ExtractEdges([]byte(data))
	want := [][2]string{
		{"a", "b"},
		{"a", "c"},
		{"b", "c"},
		{"c", "d"},
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("ExtractEdges: expecting %v, got %v", want, got)
	}
}
