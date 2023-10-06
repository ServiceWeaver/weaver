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

package run

import (
	"fmt"
	"net"
	"slices"
	"testing"

	"golang.org/x/net/context"
)

func TestConstantResolver(t *testing.T) {
	want := []string{"a", "b", "c"}
	r := ConstantResolver(want...)
	if got := <-r.Addresses(); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if addrs, ok := <-r.Addresses(); ok {
		// The channel should be closed.
		t.Errorf("unexpected addresses: %v", addrs)
	}
	if r.Err() != nil {
		t.Errorf("unexpected error: %v", r.Err())
	}
}

func TestDNSResolver(t *testing.T) {
	want, err := net.LookupHost("localhost")
	if err != nil {
		t.Fatal(err)
	}
	for i, ip := range want {
		want[i] = net.JoinHostPort(ip, "8000")
	}

	r, err := DNSResolver(context.Background(), "localhost:8000")
	if err != nil {
		t.Fatal(err)
	}
	if got := <-r.Addresses(); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCancelDNSResolver(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r, err := DNSResolver(ctx, "localhost:8000")
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	<-r.Addresses()
	if addrs, ok := <-r.Addresses(); ok {
		// The channel should be closed.
		t.Errorf("unexpected addresses: %v", addrs)
	}
	if r.Err() != ctx.Err() {
		t.Errorf("unexpected error: got %v, want %v", r.Err(), ctx.Err())
	}
}

func TestCombinedConstantResolvers(t *testing.T) {
	a := ConstantResolver("a")
	b := ConstantResolver("b")
	c := ConstantResolver("c")
	r := CombinedResolver(context.Background(), a, b, c)

	// The first two sets of addresses should be subsets of {"a", "b", "c"}.
	for i := 0; i < 2; i++ {
		addrs := <-r.Addresses()
		for _, addr := range addrs {
			if addr != "a" && addr != "b" && addr != "c" {
				t.Fatalf("invalid addresses: %v", addrs)
			}
		}
	}

	// The third sets of addresses should be {"a", "b", "c"}.
	if got, want := <-r.Addresses(), []string{"a", "b", "c"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	// The addresses channel should then be closed with a nil error.
	if addrs, ok := <-r.Addresses(); ok {
		t.Errorf("unexpected addresses: %v", addrs)
	}
	if r.Err() != nil {
		t.Errorf("unexpected error: %v", r.Err())
	}
}

func TestCombinedResolver(t *testing.T) {
	a := ConstantResolver("a")
	b := &resolver{addresses: make(chan []string, 10)}
	c := &resolver{addresses: make(chan []string, 10)}
	r := CombinedResolver(context.Background(), a, b, c)

	if got, want := <-r.Addresses(), []string{"a"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	b.addresses <- []string{"b1"}
	if got, want := <-r.Addresses(), []string{"a", "b1"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	c.addresses <- []string{"c1"}
	if got, want := <-r.Addresses(), []string{"a", "b1", "c1"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	b.addresses <- []string{"b2"}
	if got, want := <-r.Addresses(), []string{"a", "b2", "c1"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	// The channel should be closed with a nil error.
	b.close(nil)
	c.close(nil)
	if addrs, ok := <-r.Addresses(); ok {
		t.Errorf("unexpected addresses: %v", addrs)
	}
	if r.Err() != nil {
		t.Errorf("unexpected error: %v", r.Err())
	}
}

func TestCombinedResolverPropagatesErrors(t *testing.T) {
	a := ConstantResolver("a")
	b := &resolver{addresses: make(chan []string, 10)}
	c := &resolver{addresses: make(chan []string, 10)}
	r := CombinedResolver(context.Background(), a, b, c)

	// Update the addresses.
	if got, want := <-r.Addresses(), []string{"a"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	b.addresses <- []string{"b1"}
	if got, want := <-r.Addresses(), []string{"a", "b1"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	c.addresses <- []string{"c1"}
	if got, want := <-r.Addresses(), []string{"a", "b1", "c1"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	// Error out b. The error should be returned by the combined resolver.
	err := fmt.Errorf("want")
	b.close(err)
	if addrs, ok := <-r.Addresses(); ok {
		t.Errorf("unexpected addresses: %v", addrs)
	}
	if r.Err() != err {
		t.Errorf("unexpected error: got %v, want %v", r.Err(), err)
	}
}

func TestCancelledCombinedResolver(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	a := &resolver{addresses: make(chan []string, 1)}
	r := CombinedResolver(ctx, a)

	// Update the resolver three times, and then cancel the context.
	a.addresses <- []string{"a1"}
	a.addresses <- []string{"a2"}
	a.addresses <- []string{"a3"}
	cancel()

	// The resolver should return the three addresses and then be closed.
	<-r.Addresses()
	<-r.Addresses()
	<-r.Addresses()
	if addrs, ok := <-r.Addresses(); ok {
		t.Errorf("unexpected addresses: %v", addrs)
	}
	if r.Err() != ctx.Err() {
		t.Errorf("unexpected error: got %v, want %v", r.Err(), ctx.Err())
	}
}
