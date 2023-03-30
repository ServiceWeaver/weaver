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

package weaver

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/internal/net/call"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/go-cmp/cmp"
)

// nilEndpoint is a call.Endpoint that returns nil network connections.
type nilEndpoint struct{ Addr string }

// Dial implements the call.Endpoint interface.
func (nilEndpoint) Dial(context.Context) (net.Conn, error) {
	return nil, fmt.Errorf("unimplemented")
}

// Address implements the call.Endpoint interface.
func (ne nilEndpoint) Address() string {
	return ne.Addr
}

// TestRoutingBalancerNoAssignment tests that a routingBalancer with no
// assignment will use its default balancer instead.
func TestRoutingBalancerNoAssignment(t *testing.T) {
	for _, opts := range []call.CallOptions{
		{},
		{ShardKey: 1},
	} {
		t.Run(fmt.Sprint(opts.ShardKey), func(t *testing.T) {
			b := call.BalancerFunc(func([]call.Endpoint, call.CallOptions) (call.Endpoint, error) {
				return nilEndpoint{"a"}, nil
			})
			rb := routingBalancer{balancer: b}
			got, err := rb.Pick(opts)
			if err != nil {
				t.Fatal(err)
			}
			if want := (nilEndpoint{"a"}); got != want {
				t.Fatalf("rb.Pick(%v): got %v, want %v", opts, got, want)
			}
		})
	}
}

// TestRoutingBalancer tests that a routingBalancer with an assignment will
// pick endpoints using its assignment.
func TestRoutingBalancer(t *testing.T) {
	b := call.BalancerFunc(func([]call.Endpoint, call.CallOptions) (call.Endpoint, error) {
		return nil, fmt.Errorf("default balancer called")
	})
	rb := routingBalancer{balancer: b}

	assignment := &protos.Assignment{
		Slices: []*protos.Assignment_Slice{
			{
				Start:    0,
				Replicas: []string{"tcp://a"},
			},
			{
				Start:    100,
				Replicas: []string{"tcp://b"},
			},
		},
	}
	rb.update(assignment)

	for _, test := range []struct {
		shardKey uint64
		want     call.NetEndpoint
	}{
		{20, call.TCP("a")},
		{120, call.TCP("b")},
	} {
		t.Run(fmt.Sprint(test.shardKey), func(t *testing.T) {
			got, err := rb.Pick(call.CallOptions{ShardKey: test.shardKey})
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("rb.Pick(%d): got %v, want %v", test.shardKey, got, test.want)
			}
		})
	}
}

// TestRoutingResolverInitialResolve tests that the first Resolve invocation on
// a routingResolver returns a nil set of endpoints but a non-nil version.
func TestRoutingResolverInitialResolve(t *testing.T) {
	ctx := context.Background()
	r := newRoutingResolver()
	endpoints, version, err := r.Resolve(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if endpoints != nil {
		t.Fatalf("endpoints: got %v, want %v", endpoints, nil)
	}
	if version == nil {
		t.Fatal("unexpected nil version")
	}
}

// TestRoutingResolverUpdate tests that the endpoints and versions passed to
// Update (either synchronously or asynchronously) are correctly returned by a
// call to Resolve.
func TestRoutingResolverUpdate(t *testing.T) {
	ab := []call.Endpoint{nilEndpoint{"a"}, nilEndpoint{"b"}}
	cd := []call.Endpoint{nilEndpoint{"c"}, nilEndpoint{"d"}}

	// Update synchronously.
	ctx := context.Background()
	r := newRoutingResolver()
	r.update(ab)
	endpoints, version, err := r.Resolve(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(endpoints, ab); diff != "" {
		t.Fatalf("endpoints (-want +got):\n%s", diff)
	}

	// Update asynchronously.
	go func() {
		time.Sleep(25 * time.Millisecond)
		r.update(cd)
	}()
	endpoints, _, err = r.Resolve(ctx, version)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(endpoints, cd); diff != "" {
		t.Fatalf("endpoints (-want +got):\n%s", diff)
	}
}
