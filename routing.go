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
	"crypto/tls"
	"math/rand"
	"sync"

	"github.com/ServiceWeaver/weaver/internal/cond"
	"github.com/ServiceWeaver/weaver/internal/net/call"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/uuid"
)

// routingBalancer balances requests according to a routing assignment.
type routingBalancer struct {
	balancer  call.Balancer // default balancer
	tlsConfig *tls.Config   // tls config to use; may be nil.

	mu         sync.RWMutex
	assignment *protos.Assignment
	index      index
}

// newRoutingBalancer returns a new routingBalancer.
func newRoutingBalancer(tlsConfig *tls.Config) *routingBalancer {
	return &routingBalancer{balancer: call.RoundRobin(), tlsConfig: tlsConfig}
}

// Update implements the call.Balancer interface.
func (rb *routingBalancer) Update(endpoints []call.Endpoint) {
	rb.balancer.Update(endpoints)
}

// update updates the balancer with the provided assignment
func (rb *routingBalancer) update(assignment *protos.Assignment) {
	if assignment == nil {
		return
	}

	index := newIndex(assignment)
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.assignment = assignment
	rb.index = index
}

// Pick implements the call.Balancer interface.
func (rb *routingBalancer) Pick(opts call.CallOptions) (call.Endpoint, error) {
	if opts.ShardKey == 0 {
		// If the method we're calling is not sharded (which is guaranteed to
		// be true for nonsharded components), then the shard key is 0.
		return rb.balancer.Pick(opts)
	}

	// Grab the current assignment. It's possible that the current assignment
	// changes between when we release the lock and when we pick an endpoint,
	// but using a slightly stale assignment is okay.
	rb.mu.RLock()
	assignment := rb.assignment
	index := rb.index
	rb.mu.RUnlock()

	if assignment == nil {
		// There is no assignment. This is possible if we haven't received an
		// assignment from the assigner yet.
		return rb.balancer.Pick(opts)
	}

	slice, ok := index.find(opts.ShardKey)
	if !ok {
		// TODO(mwhittaker): Shouldn't this be impossible. Understand better
		// when this happens.
		return rb.balancer.Pick(opts)
	}

	// TODO(mwhittaker): Double check that the endpoint in the slice is one of
	// the endpoints in rb.endpoints.
	//
	// TODO(mwhittaker): Parse the endpoints when an assignment is received,
	// rather than once per call.
	addr := slice.replicas[rand.Intn(len(slice.replicas))]
	endpoints, err := parseEndpoints([]string{addr}, rb.tlsConfig)
	if err != nil {
		return nil, err
	}
	return endpoints[0], nil
}

// routingResolver is a dummy resolver that returns whatever endpoints are
// passed to the update method.
type routingResolver struct {
	m         sync.Mutex      // guards all of the following fields
	changed   cond.Cond       // fires when endpoints changes
	version   *call.Version   // the current version of endpoints
	endpoints []call.Endpoint // the endpoints returned by Resolve
}

// newRoutingResolver returns a new routingResolver.
func newRoutingResolver() *routingResolver {
	r := &routingResolver{
		version: &call.Version{Opaque: call.Missing.Opaque},
	}
	r.changed.L = &r.m
	return r
}

// IsConstant implements the call.Resolver interface.
func (rr *routingResolver) IsConstant() bool { return false }

// update updates the resolver with the provided endpoints.
func (rr *routingResolver) update(endpoints []call.Endpoint) {
	rr.m.Lock()
	defer rr.m.Unlock()
	rr.version = &call.Version{Opaque: uuid.New().String()}
	rr.endpoints = endpoints
	rr.changed.Broadcast()
}

// Resolve implements the call.Resolver interface.
func (rr *routingResolver) Resolve(ctx context.Context, version *call.Version) ([]call.Endpoint, *call.Version, error) {
	rr.m.Lock()
	defer rr.m.Unlock()

	if version == nil {
		return rr.endpoints, rr.version, nil
	}

	for *version == *rr.version {
		if err := rr.changed.Wait(ctx); err != nil {
			return nil, nil, err
		}
	}
	return rr.endpoints, rr.version, nil
}
