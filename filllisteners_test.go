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
	"fmt"
	"net"
	"strings"
	"testing"
)

type testListener struct {
	net.Listener
}

func getListener(lis string) (net.Listener, string, error) {
	if lis != "a" && lis != "b" && lis != "cname" && lis != "dname" {
		return nil, "", fmt.Errorf("unexpected listener %q", lis)
	}
	return &testListener{}, lis, nil
}

func TestFillListeners(t *testing.T) {
	var x struct {
		A Listener
		b Listener
		C Listener `weaver:"cname"`
		d Listener `weaver:"DName"`
	}
	if err := fillListeners(&x, getListener); err != nil {
		t.Fatal(err)
	}
	if x.A.proxyAddr != "a" {
		t.Errorf(`expecting x.A.proxyAddr to be "a", got %q`, x.A.proxyAddr)
	}
	if x.b.proxyAddr != "b" {
		t.Errorf(`expecting x.b.proxyAddr to be "b", got %q`, x.b.proxyAddr)
	}
	if x.C.proxyAddr != "cname" {
		t.Errorf(`expecting x.C.proxyAddr to be "cname", got %q`, x.C.proxyAddr)
	}
	if x.d.proxyAddr != "dname" {
		t.Errorf(`expecting x.d.proxyAddr to be "dname", got %q`, x.d.proxyAddr)
	}
}

func TestFillListenerErrors(t *testing.T) {
	type impl struct{}
	type testCase struct {
		name   string
		impl   any    // impl argument to pass to fillRefs
		expect string // Returned error must contain this string
	}
	for _, c := range []testCase{
		{"not-pointer", impl{}, "not a pointer"},
		{"not-struct-pointer", new(int), "not a struct pointer"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := fillRefs(c.impl, getValue)
			if err == nil || !strings.Contains(err.Error(), c.expect) {
				t.Fatalf("unexpected error %v; expecting %s", err, c.expect)
			}
		})
	}
}
