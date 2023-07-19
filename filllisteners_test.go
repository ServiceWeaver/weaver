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
	"testing"
)

type testListener struct {
	net.Listener
}

func getListener(lis string) (net.Listener, string, error) {
	if lis != "A" && lis != "b" && lis != "cname" && lis != "DName" {
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
	if x.A.ProxyAddr() != "A" {
		t.Errorf(`expecting x.A.ProxyAddr() to be "A", got %q`, x.A.ProxyAddr())
	}
	if x.b.ProxyAddr() != "b" {
		t.Errorf(`expecting x.b.ProxyAddr() to be "b", got %q`, x.b.ProxyAddr())
	}
	if x.C.ProxyAddr() != "cname" {
		t.Errorf(`expecting x.C.ProxyAddr() to be "cname", got %q`, x.C.ProxyAddr())
	}
	if x.d.ProxyAddr() != "DName" {
		t.Errorf(`expecting x.d.ProxyAddr() to be "Dname", got %q`, x.d.ProxyAddr())
	}
}
