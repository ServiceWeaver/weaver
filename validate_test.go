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

package weaver

import (
	"io"
	"strings"
	"testing"

	"github.com/ServiceWeaver/weaver/internal/reflection"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
)

// TestValidateNoRegistrations tests that validateRegistrations succeeds on an
// empty set of registrations.
func TestValidateNoRegistrations(t *testing.T) {
	if err := validateRegistrations(nil); err != nil {
		t.Fatal(err)
	}
}

// TestValidateValidRegistrations tests that validateRegistrations succeeds on
// a set of valid registrations.
func TestValidateValidRegistrations(t *testing.T) {
	type foo struct{}
	type bar struct{}
	type fooImpl struct{ Ref[bar] }
	type barImpl struct{ Ref[foo] }
	regs := []*codegen.Registration{
		{
			Name:  "foo",
			Iface: reflection.Type[foo](),
			Impl:  reflection.Type[fooImpl](),
		},
		{
			Name:  "bar",
			Iface: reflection.Type[bar](),
			Impl:  reflection.Type[barImpl](),
		},
	}
	if err := validateRegistrations(regs); err != nil {
		t.Fatal(err)
	}
}

// TestValidateUnregisteredRef tests that validateRegistrations fails when a
// component has a weaver.Ref on an unregistered component.
func TestValidateUnregisteredRef(t *testing.T) {
	type foo struct{}
	type fooImpl struct{ Ref[io.Reader] }
	regs := []*codegen.Registration{
		{
			Name:  "foo",
			Iface: reflection.Type[foo](),
			Impl:  reflection.Type[fooImpl](),
		},
	}
	err := validateRegistrations(regs)
	if err == nil {
		t.Fatal("unexpected validateRegistrations success")
	}
	const want = "component io.Reader was not registered"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("validateRegistrations: got %q, want %q", err, want)
	}
}
