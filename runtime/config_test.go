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

package runtime_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestBinaryPath(t *testing.T) {
	type testCase struct {
		name   string
		dir    string
		binary string
		expect string
	}

	cwd, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf(`filepath.Abs("."): %v`, err)
	}
	for _, c := range []testCase{
		{"Relative/Relative", ".", "./foo", filepath.Join(cwd, "foo")},
		{"Relative/Abs", ".", "/tmp/foo", "/tmp/foo"},
		{"Abs/Relative", "/bin", "./foo", "/bin/foo"},
		{"Abs/Abs", "/bin", "/tmp/foo", "/tmp/foo"},
	} {
		t.Run(c.name, func(t *testing.T) {
			spec := fmt.Sprintf("[serviceweaver]\nbinary = '%s'\n", c.binary)
			cfgFile := filepath.Join(c.dir, "weaver.toml")
			cfg, err := runtime.ParseConfig(cfgFile, spec, codegen.ComponentConfigValidator)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if got, want := cfg.Binary, c.expect; got != want {
				t.Fatalf("binary: got %q, want %q", got, want)
			}
		})
	}
}

func TestParseConfigSection(t *testing.T) {
	type section struct {
		Foo string
		Bar string
		Baz int
	}
	type testCase struct {
		name         string
		initialValue section
		config       string
		expect       section
	}
	for _, c := range []testCase{
		{"missing", section{}, ``, section{}},
		{"empty", section{}, "[section]\n", section{}},
		{
			"full",
			section{},
			`section = { Foo = "foo", Bar = "bar", Baz = 100 }`,
			section{"foo", "bar", 100},
		},
		{
			"partial",
			section{Baz: 200},
			`section = {Foo = "foo", Bar = "bar" }`,
			section{"foo", "bar", 200},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			config, err := runtime.ParseConfig("", c.config, codegen.ComponentConfigValidator)
			if err != nil {
				t.Fatal(err)
			}
			got := c.initialValue
			err = runtime.ParseConfigSection("section", "", config.Sections, &got)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(c.expect, got); diff != "" {
				t.Fatalf("ParseConfigSection: (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseListeners(t *testing.T) {
	type testCase struct {
		name     string
		config   string
		expected []string
	}
	for _, c := range []testCase{
		{
			name: "short_separate",
			config: `
[listeners.foo]
local_address = ":1"

[listeners.bar]
local_address = ":2"
`,
			expected: []string{"foo :1", "bar :2"},
		},
		{
			name: "short_together",
			config: `
[listeners]
foo = {local_address = ":1"}
bar = {local_address = ":2"}
`,
			expected: []string{"foo :1", "bar :2"},
		},
		{
			name: "short_inlined",
			config: `
listeners.foo.local_address = ":1"
listeners.bar.local_address = ":2"
`,
			expected: []string{"foo :1", "bar :2"},
		},
		{
			name: "long",
			config: `
["github.com/ServiceWeaver/weaver/listeners".foo]
local_address = ":1"

["github.com/ServiceWeaver/weaver/listeners".bar]
local_address = ":2"
`,
			expected: []string{"foo :1", "bar :2"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			config, err := runtime.ParseConfig("", c.config, codegen.ComponentConfigValidator)
			if err != nil {
				t.Fatal(err)
			}
			var actual []string
			for name, opts := range config.ListenerOptions {
				actual = append(actual, fmt.Sprintf("%s %s", name, opts.LocalAddress))
			}
			less := func(x, y string) bool { return x < y }
			if diff := cmp.Diff(c.expected, actual, cmpopts.SortSlices(less)); diff != "" {
				t.Fatalf("(-want +got):\n%s", diff)
			}
		})
	}
}

func TestConfigErrors(t *testing.T) {
	type testCase struct {
		name          string
		cfg           string
		expectedError string
	}
	for _, c := range []testCase{
		{
			name: "same-process-inter-group-conflict",
			cfg: `
[serviceweaver]
colocate = [["a", "main"], ["a", "c"]]
`,
			expectedError: "placed multiple times",
		},
		{
			name: "same-process-intra-group-conflict",
			cfg: `
[serviceweaver]
colocate = [["a", "main", "a"]]
`,
			expectedError: "placed multiple times",
		},
		{
			name: "conflicting sections",
			cfg: `
[serviceweaver]
name = "foo"

["github.com/ServiceWeaver/weaver"]
binary = "/tmp/foo"
`,
			expectedError: "conflicting",
		},
		{
			name: "conflicting listener sections",
			cfg: `
[listeners]
foo = {local_address = ":1"}

["github.com/ServiceWeaver/weaver/listeners"]
bar = {local_address = ":2"}
`,
			expectedError: "conflicting",
		},
		{
			name: "unknown key",
			cfg: `
[serviceweaver]
badkey = "foo"
`,
			expectedError: "unknown",
		},
		{
			name: "bad rollout",
			cfg: `
[serviceweaver]
rollout = "hello"
`,
			expectedError: "invalid duration",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := runtime.ParseConfig("weaver.toml", c.cfg, codegen.ComponentConfigValidator)
			if err == nil {
				t.Fatalf("unexpected success when expecting %q in\n%s", c.expectedError, c.cfg)
			}
			if !strings.Contains(err.Error(), c.expectedError) {
				t.Fatalf("error %v does not contain %q in\n%s", err, c.expectedError, c.cfg)
			}
		})
	}
}
