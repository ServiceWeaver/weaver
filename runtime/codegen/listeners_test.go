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

func TestListeners(t *testing.T) {
	c := codegen.MakeListenersString("c", []string{"l5", "l4"})
	b := codegen.MakeListenersString("b", []string{"l3"})
	a := codegen.MakeListenersString("a", []string{"l1", "l2"})
	data := a + b + c
	t.Log(data)

	got := codegen.ExtractListeners([]byte(data))
	want := []codegen.ComponentListeners{
		{"a", []string{"l1", "l2"}},
		{"b", []string{"l3"}},
		{"c", []string{"l4", "l5"}},
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("ExtractEdges: expecting %v, got %v", want, got)
	}
}
