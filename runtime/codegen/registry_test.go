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
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"go.opentelemetry.io/otel/trace"
)

func TestComponentConfigValidator(t *testing.T) {
	if err := codegen.ComponentConfigValidator(typeWithConfig, `Foo = "hello"`); err != nil {
		t.Fatal(err)
	}
}

func TestComponentConfigValidatorErrors(t *testing.T) {
	type testcase struct {
		config        string
		path          string
		expectedError string
	}
	for _, test := range []testcase{
		{
			path:          typeWithoutConfig,
			config:        `Foo = "hello"`,
			expectedError: "unexpected configuration",
		},
		{
			path:          typeWithConfig,
			config:        `Bar = "hello"`,
			expectedError: "incompatible types",
		},
		{
			path:          typeWithConfig,
			config:        `Bar = -100`,
			expectedError: "invalid value",
		},
	} {
		t.Run(test.expectedError, func(t *testing.T) {
			err := codegen.ComponentConfigValidator(test.path, test.config)
			if err == nil {
				t.Fatal("unexpected success")
			}
			if !strings.Contains(err.Error(), test.expectedError) {
				t.Fatalf("unexpected error %v", err)
			}
		})
	}
}

const (
	typeWithoutConfig = "codegen_test/withoutConfig"
	typeWithConfig    = "codegen_test/withConfig"
)

type componentWithoutConfig struct{}

type componentWithConfig struct {
	weaver.WithConfig[testconfig]
}

type testconfig struct {
	Foo string
	Bar int
}

func (t testconfig) Validate() error {
	if t.Bar < 0 {
		return fmt.Errorf("invalid value")
	}
	return nil
}

// Register dummy components for test.
func init() {
	local := func(any, trace.Tracer) any { return nil }
	client := func(codegen.Stub, string) any { return nil }
	server := func(any, func(uint64, float64)) codegen.Server { return nil }

	codegen.Register(codegen.Registration{
		Name:         typeWithoutConfig,
		Iface:        reflect.TypeOf((*componentWithoutConfig)(nil)).Elem(),
		New:          func() any { return &componentWithoutConfig{} },
		LocalStubFn:  local,
		ClientStubFn: client,
		ServerStubFn: server,
	})
	codegen.Register(codegen.Registration{
		Name:         typeWithConfig,
		Iface:        reflect.TypeOf((*componentWithConfig)(nil)).Elem(),
		New:          func() any { return &componentWithConfig{} },
		ConfigFn:     func(i any) any { return i.(*componentWithConfig).Config() },
		LocalStubFn:  local,
		ClientStubFn: client,
		ServerStubFn: server,
	})
}
