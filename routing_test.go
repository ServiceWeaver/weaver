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
	return nil, fmt.Errorf("unimpemented")
}

// Address implements the call.Endpoint interface.
func (ne nilEndpoint) Address() string {
	return ne.Addr
}

// TestRoutelet tests that a routelet correctly updates its returned resolvers
// and balancers whenever its routing information is updated.
func TestRoutelet(t *testing.T) {
	// Don't call newRoutelet; we don't want to spawn the watching goroutine.
	r := routelet{}
	rr := r.resolver()
	rb := r.balancer()

	info := &protos.RoutingInfo{
		Unchanged: false,
		Version:   "1",
		Replicas:  []string{"tcp://a", "tcp://b"},
		Assignment: &protos.Assignment{
			Slices: []*protos.Assignment_Slice{
				{Start: 0, Replicas: []string{"tcp://a"}},
				{Start: 100, Replicas: []string{"tcp://b"}},
			},
			App:          "app",
			DeploymentId: "id",
			Component:    "Foo",
			Version:      1,
		},
	}
	v1 := &call.Version{Opaque: "1"}
	if err := r.update(info, v1); err != nil {
		t.Fatal(err)
	}

	// Check rr.
	ctx := context.Background()
	endpoints, version, err := rr.Resolve(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(endpoints, []call.Endpoint{call.TCP("a"), call.TCP("b")}); diff != "" {
		t.Fatalf("rr.Resolve (-want +got):\n%s", diff)
	}
	if version == nil || *version != *v1 {
		t.Fatalf("rr.Resolve: got %v, want %v", version, v1)
	}

	// Check rbFoo.
	endpoint, err := rb.Pick(call.CallOptions{ShardKey: 120})
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != call.TCP("b") {
		t.Fatalf("rbFoo.Pick: got %v, want %v", endpoint, call.TCP("b"))
	}
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
	rb.updateAssignment(assignment)

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
	v0 := call.Version{Opaque: "0"}
	v1 := call.Version{Opaque: "1"}

	// Update synchronously.
	ctx := context.Background()
	r := newRoutingResolver()
	r.update(ab, &v0)
	endpoints, version, err := r.Resolve(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(endpoints, ab); diff != "" {
		t.Fatalf("endpoints (-want +got):\n%s", diff)
	}
	if version == nil && *version != v0 {
		t.Fatalf("version: got %v, want %v", version, &v0)
	}

	// Update asynchronously.
	go func() {
		time.Sleep(25 * time.Millisecond)
		r.update(cd, &v1)
	}()
	endpoints, version, err = r.Resolve(ctx, version)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(endpoints, cd); diff != "" {
		t.Fatalf("endpoints (-want +got):\n%s", diff)
	}
	if version == nil || *version != v1 {
		t.Fatalf("version: got %v, want %v", version, &v1)
	}
}
