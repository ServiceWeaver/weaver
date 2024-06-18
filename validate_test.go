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
	type foo interface{}
	type bar interface{}
	type fooImpl struct {
		Ref[bar]
		Listener `weaver:"lis1"`
		_        Listener `weaver:"lis2"`
		lis3     Listener //lint:ignore U1000 Present for code generation.
	}
	type barImpl struct{ Ref[foo] }
	regs := []*codegen.Registration{
		{
			Name:      "foo",
			Iface:     reflection.Type[foo](),
			Impl:      reflection.Type[fooImpl](),
			Listeners: []string{"lis1", "lis2", "lis3"},
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
	type foo interface{}
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

// TestValidateInvalidListenerNames tests that validateRegistrations fails on
// invalid listener names.
func TestValidateInvalidListenerNames(t *testing.T) {
	type foo interface{}
	type fooImpl struct {
		_ Listener `weaver:""`             // empty name
		_ Listener `weaver:" "`            // whitespace name
		_ Listener `weaver:"foo bar"`      // whitespace in name
		_ Listener `weaver:"1foo"`         // starts with a digit
		_ Listener `weaver:".!@#$%^&*()-"` // punctuation
	}
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
	for _, want := range []string{
		`invalid listener tag ""`,
		`invalid listener tag " "`,
		`invalid listener tag "foo bar"`,
		`invalid listener tag "1foo"`,
		`invalid listener tag ".!@#$%^&*()-"`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("validateRegistrations: got %q, want %q", err, want)
		}
	}
}

// TestValidateUnregisteredListeners tests that validateRegistrations fails on
// listener names that haven't been registered.
func TestValidateUnregisteredListener(t *testing.T) {
	type foo interface{}
	type fooImpl struct {
		foo Listener //lint:ignore U1000 Present for code generation.
		bar Listener //lint:ignore U1000 Present for code generation.
		baz Listener //lint:ignore U1000 Present for code generation.
	}

	regs := []*codegen.Registration{
		{
			Name:      "foo",
			Iface:     reflection.Type[foo](),
			Impl:      reflection.Type[fooImpl](),
			Listeners: []string{"foo"},
		},
	}
	err := validateRegistrations(regs)
	if err == nil {
		t.Fatal("unexpected validateRegistrations success")
	}
	for _, want := range []string{
		`listener bar hasn't been registered`,
		`listener baz hasn't been registered`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("validateRegistrations: got %q, want %q", err, want)
		}
	}
}

func TestValidate_checkCircularDependencys(t *testing.T) {
	type foo interface{}
	type fooImpl struct {
		Ref[foo]
	}

	regs := []*codegen.Registration{
		{
			Name:    "github.com/ServiceWeaver/weaver/Main",
			Iface:   reflection.Type[foo](),
			Impl:    reflection.Type[fooImpl](),
			RefData: `⟦b8422d0e:KoDeDgE:github.com/ServiceWeaver/weaver/test→github.com/ServiceWeaver/weaver/test1Controller`,
		},
		{
			Name:    "github.com/ServiceWeaver/weaver/test1Controller",
			Iface:   reflection.Type[foo](),
			Impl:    reflection.Type[fooImpl](),
			RefData: `⟦b8422d0e:KoDeDgE:github.com/ServiceWeaver/weaver/test1Controller→github.com/ServiceWeaver/weaver/Main⟧`,
		},
	}

	if err := checkCircularDependency(regs); err != nil {
		t.Fatal(err)
	}
}

func TestValidate_checkCircularDependencysError(t *testing.T) {
	type LocalCache interface {
	}
	type ImageScaler interface {
	}

	type implLocalCache struct {
		Implements[LocalCache]
		Ref[ImageScaler]
	}

	type scaler struct {
		Implements[ImageScaler]
		Ref[LocalCache]
	}

	type implBFF struct {
		Implements[Main]
		Ref[ImageScaler]
	}

	regs := []*codegen.Registration{
		{
			Name:    "github.com/renanbastos93/weaver-issue-556/ImageScaler",
			Iface:   reflection.Type[ImageScaler](),
			Impl:    reflection.Type[scaler](),
			RefData: `⟦965985aa:wEaVeReDgE:github.com/renanbastos93/weaver-issue-556/ImageScaler→github.com/renanbastos93/weaver-issue-556/LocalCache⟧`,
		},
		{
			Name:    "github.com/renanbastos93/weaver-issue-556/LocalCache",
			Iface:   reflection.Type[LocalCache](),
			Impl:    reflection.Type[implLocalCache](),
			RefData: `⟦0a9731a1:wEaVeReDgE:github.com/renanbastos93/weaver-issue-556/LocalCache→github.com/renanbastos93/weaver-issue-556/ImageScaler⟧`,
		},
		{
			Name:    "github.com/ServiceWeaver/weaver/Main",
			Iface:   reflection.Type[Main](),
			Impl:    reflection.Type[implBFF](),
			RefData: `⟦50668105:wEaVeReDgE:github.com/ServiceWeaver/weaver/Main→github.com/renanbastos93/weaver-issue-556/ImageScaler`,
		},
	}

	if err := checkCircularDependency(regs); err != nil {
		const want = "components have cycle Ref"
		if err.Error() != want {
			t.Fatalf("validateRegistrations: got %q, want %q", err, want)
		}
	} else {
		t.Fatal("unexpected checkCircularDependency success")
	}

}
