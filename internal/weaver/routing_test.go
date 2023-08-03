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

// fakeConn is a fake call.ReplicaConnection used for testing.
type fakeConn string

func (f fakeConn) Address() string { return string(f) }

// TestRoutingBalancerNoAssignment tests that a routingBalancer with no
// assignment will use its default balancer instead.
func TestRoutingBalancerNoAssignment(t *testing.T) {
	for _, opts := range []call.CallOptions{
		{},
		{ShardKey: 1},
	} {
		t.Run(fmt.Sprint(opts.ShardKey), func(t *testing.T) {
			b := call.BalancerFunc(func([]call.ReplicaConnection, call.CallOptions) (call.ReplicaConnection, bool) {
				return fakeConn("a"), false
			})
			rb := routingBalancer{balancer: b}
			if got, ok := rb.Pick(opts); ok {
				t.Fatalf("r.Pick unexpectedly returned %s", got.Address())
			}
		})
	}
}

// TestRoutingBalancer tests that a routingBalancer with an assignment will
// pick endpoints using its assignment.
func TestRoutingBalancer(t *testing.T) {
	b := call.BalancerFunc(func([]call.ReplicaConnection, call.CallOptions) (call.ReplicaConnection, bool) {
		t.Fatal("default balancer called")
		return nil, false
	})
	rb := routingBalancer{balancer: b, conns: map[string]call.ReplicaConnection{}}

	assignment := &protos.Assignment{
		Slices: []*protos.Assignment_Slice{
			{
				Start:    0,
				Replicas: []string{"a"},
			},
			{
				Start:    100,
				Replicas: []string{"b"},
			},
		},
	}
	rb.update(assignment)
	rb.Add(fakeConn("a"))
	rb.Add(fakeConn("b"))

	for _, test := range []struct {
		shardKey uint64
		want     string
	}{
		{20, "a"},
		{120, "b"},
	} {
		t.Run(fmt.Sprint(test.shardKey), func(t *testing.T) {
			got, ok := rb.Pick(call.CallOptions{ShardKey: test.shardKey})
			if !ok {
				t.Fatal("did not find replica")
			}
			if got.Address() != test.want {
				t.Fatalf("rb.Pick(%d): got %s, want %s", test.shardKey, got.Address(), test.want)
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
