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
	"math/rand"
	"sync"

	"github.com/ServiceWeaver/weaver/internal/cond"
	"github.com/ServiceWeaver/weaver/internal/net/call"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/retry"
)

// routelet Implementation Details
//
// When a routelet is created, it spawns a goroutine that repeatedly issues
// RouteInfoRequests to the manager for a given process. These are blocking
// calls that return whenever the routing information has changed.
//
// You can use a routelet's Resolver and Balancer methods to get a
// call.Resolver and call.Balancer, respectively. Both the resolver and
// balancer are automatically updated whenever the routing information changes.

// routelet maintains the latest routing information for a process by
// communicating with the manager.
type routelet struct {
	env env

	mu          sync.Mutex                  // guards the following fields
	routingInfo *protos.RoutingInfo         // latest routing info from assigner
	res         *routingResolver            // resolver, updated with routingInfo
	balancers   map[string]*routingBalancer // balancers, keyed by component name
	callbacks   []func(*protos.RoutingInfo) // invoked when routing info changes
}

// newRoutelet returns a new routelet for the provided colocation group. The
// lifetime of the routelet is bound to the provided context. When the context is
// cancelled, the routelet stops tracking the latest routing information.
func newRoutelet(ctx context.Context, env env, group string) *routelet {
	r := &routelet{env: env, balancers: map[string]*routingBalancer{}}
	go r.watchRoutingInfo(ctx, env, group)
	return r
}

// resolver returns a new call.Resolver that returns the weavelet addresses
// that implement the process for which this routelet was created.
func (r *routelet) resolver() call.Resolver {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.res == nil {
		r.res = newRoutingResolver()
	}
	return r.res
}

// balancer returns a new call.Balancer for the provided component that uses the
// latest routing assignment to route requests. The provided component must be a
// component in the process used to create this routelet.
func (r *routelet) balancer(component string) call.Balancer {
	r.mu.Lock()
	defer r.mu.Unlock()

	balancer, ok := r.balancers[component]
	if ok {
		return balancer
	}

	// Register the balancer.
	balancer = &routingBalancer{balancer: call.RoundRobin()}
	r.balancers[component] = balancer

	// Update the balancer with its initial assignment, if any.
	if r.routingInfo == nil {
		return balancer
	}
	for _, assignment := range r.routingInfo.Assignments {
		if assignment.Component != component {
			continue
		}
		balancer.updateAssignment(assignment)
	}
	return balancer
}

// onChange registers a callback that is invoked every time the routing info
// changes.
func (r *routelet) onChange(callback func(*protos.RoutingInfo)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callbacks = append(r.callbacks, callback)
}

// watchRoutingInfo repeatedly issues blocking RoutingInfoRequests to the
// manager to track the latest routing info. Whenever the routing info changes,
// any resolvers and balancers returned by the Resolver() and Balancer()
// methods are updated.
func (r *routelet) watchRoutingInfo(ctx context.Context, env env, group string) error {
	var version *call.Version
	for re := retry.Begin(); re.Continue(ctx); {
		routingInfo, newVersion, err := env.GetRoutingInfo(ctx, group, version)
		if err != nil {
			// TODO(mwhittaker): Handle errors more gracefully.
			r.env.SystemLogger().Error("cannot get routing info", err)
			continue
		}
		version = newVersion
		if err := r.update(routingInfo, version); err != nil {
			// TODO(mwhittaker): Handle errors more gracefully.
			r.env.SystemLogger().Error("cannot update routing info", err)
			continue
		}
		re.Reset()
	}
	return ctx.Err()
}

// update updates the state of a routelet with the latest routing info.
func (r *routelet) update(routingInfo *protos.RoutingInfo, version *call.Version) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Update routing info.
	r.routingInfo = routingInfo

	// Update resolver.
	if r.res != nil {
		endpoints, err := parseEndpoints(routingInfo.Replicas)
		if err != nil {
			return err
		}
		r.res.update(endpoints, version)
	}

	// Update balancers.
	for _, assignment := range routingInfo.Assignments {
		balancer, ok := r.balancers[assignment.Component]
		if !ok {
			continue
		}
		balancer.updateAssignment(assignment)
	}

	// Invoke callbacks.
	for _, callback := range r.callbacks {
		callback(routingInfo)
	}

	return nil
}

// routingBalancer is a load balancer that uses a routing assignment to route
// requests. While routingBalancer is a standalone entity, you should likely
// not interact with routingBalancer directly and instead use a routelet.
type routingBalancer struct {
	balancer call.Balancer // default balancer

	mu         sync.RWMutex
	assignment *protos.Assignment
	index      index
}

// Update implements the call.Balancer interface.
func (rb *routingBalancer) Update(endpoints []call.Endpoint) {
	rb.balancer.Update(endpoints)
}

// updateAssignment updates the balancer with the provided assignment.
func (rb *routingBalancer) updateAssignment(assignment *protos.Assignment) {
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
	endpoints, err := parseEndpoints([]string{addr})
	if err != nil {
		return nil, err
	}
	return endpoints[0], nil
}

// routingResolver is a dummy resolver that returns whatever endpoints are
// passed to the Update method. While routingResolver is a standalone entity,
// you should likely not interact with routingResolver directly and instead use
// a routelet.
type routingResolver struct {
	m         sync.Mutex      // guards all of the following fields
	changed   cond.Cond       // fires when endpoints changes
	version   *call.Version   // the current version of endpoints
	endpoints []call.Endpoint // the endpoints returned by Resolve
}

func newRoutingResolver() *routingResolver {
	r := &routingResolver{
		version: &call.Version{Opaque: call.Missing.Opaque},
	}
	r.changed.L = &r.m
	return r
}

// IsConstant implements the call.Resolver interface.
func (rr *routingResolver) IsConstant() bool { return false }

// update updates the set of endpoints that the resolver returns. The provided
// version must be newer than any previously provided version.
func (rr *routingResolver) update(endpoints []call.Endpoint, version *call.Version) {
	rr.m.Lock()
	defer rr.m.Unlock()
	rr.version = version
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
