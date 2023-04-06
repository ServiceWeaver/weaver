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

func TestParseNetEndpoint(t *testing.T) {
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
			netEndpoint, err := call.ParseNetEndpoint(test.s)
			if err != nil {
				t.Fatalf("%q: unexpected error: %v", test.s, err)
			}
			if got, want := netEndpoint.Net, test.network; got != want {
				t.Fatalf("%q bad network: got %q, want %q", test.s, got, want)
			}
			if got, want := netEndpoint.Addr, test.address; got != want {
				t.Fatalf("%q bad address: got %q, want %q", test.s, got, want)
			}
		})
	}
}

func TestParseNetEndpointError(t *testing.T) {
	_, err := call.ParseNetEndpoint("there is no delimiter here")
	if err == nil {
		t.Fatal("expected an error parsing invalid endpoint")
	}
}
