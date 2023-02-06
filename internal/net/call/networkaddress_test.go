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

package call_test

import (
	"testing"

	"github.com/ServiceWeaver/weaver/internal/net/call"
)

func TestSplit(t *testing.T) {
	for _, test := range []struct {
		name    string
		s       string
		network string
		address string
	}{
		{"NetworkAddress", "network://address", "network", "address"},
		{"EmptyNetwork", "://address", "", "address"},
		{"EmptyAddress", "network://", "network", ""},
		{"JustDelim", "://", "", ""},
		{"ExtraDelim", "network://a://b://c", "network", "a://b://c"},
	} {
		t.Run(test.name, func(t *testing.T) {
			network, address, err := call.NetworkAddress(test.s).Split()
			if err != nil {
				t.Fatalf("%q.Split(): unexpected error: %v", test.s, err)
			}
			if got, want := network, test.network; got != want {
				t.Fatalf("%q.Split() bad network: got %q, want %q", test.s, got, want)
			}
			if got, want := address, test.address; got != want {
				t.Fatalf("%q.Split() bad address: got %q, want %q", test.s, got, want)
			}
		})
	}
}

func TestSplitError(t *testing.T) {
	na := call.NetworkAddress("there is no delimiter here")
	_, _, err := na.Split()
	if err == nil {
		t.Fatalf("%q.Split(): unexpected success", string(na))
	}
}
