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

package logtype_test

import (
	"fmt"
	"testing"

	"github.com/ServiceWeaver/weaver/internal/logtype"
)

func TestAppendAttrs(t *testing.T) {
	list := func(args ...any) []any { return args }
	for name, test := range map[string]struct {
		prefix []string
		input  []any
		expect string
	}{
		"empty":          {nil, nil, `[]`},
		"single":         {nil, list("foo", 1), `[foo 1]`},
		"multiple":       {nil, list("foo", 1, "bar", "two"), `[foo 1 bar two]`},
		"prefixed_empty": {[]string{"prefix", "0"}, nil, `[prefix 0]`},
		"prefixed":       {[]string{"prefix", "0"}, list("foo", 1), `[prefix 0 foo 1]`},
		"odd_length":     {nil, list("foo", 1, "bar"), `[foo 1 !BADKEY bar]`},
		"missing_key":    {nil, list("foo", 1, "two"), `[foo 1 !BADKEY two]`},
	} {
		t.Run(name, func(t *testing.T) {
			attrs := logtype.AppendAttrs(test.prefix, test.input)
			got := fmt.Sprintf("%v", attrs)
			if got != test.expect {
				t.Errorf("AppendAttrs returned `%s`, expecting `%s`", got, test.expect)
			}
		})
	}

}
