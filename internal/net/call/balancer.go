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

package call

import (
	"fmt"
	"math/rand"
)

// A Balancer picks the endpoint to which which an RPC client performs a call. A
// Balancer should only be used by a single goroutine.
//
// TODO(mwhittaker): Right now, balancers have no load information about
// endpoints. In the short term, we can at least add information about the
// number of pending requests for every endpoint.
//
// TODO(mwhittaker): Right now, we pass a balancer the set of all endpoints. We
// instead probably want to pass it only the endpoints for which we have a
// connection. This means we may have to form connections more eagerly.
//
// TODO(mwhittaker): We may want to guarantee that Update() is never called
// with an empty list of addresses. If we don't have addresses, then we don't
// need to do balancing.
type Balancer interface {
	// Update updates the current set of endpoints from which the Balancer can
	// pick. Before Update is called for the first time, the set of endpoints
	// is empty.
	Update(endpoints []Endpoint)

	// Pick picks an endpoint. Pick is guaranteed to return an endpoint that
	// was passed to the most recent call of Update. If there are no endpoints,
	// then Pick returns an error that includes Unreachable.
	Pick(CallOptions) (Endpoint, error)
}

// balancerFuncImpl is the imeplementation of the "functional" balancer
// returned by BalancerFunc.
type balancerFuncImpl struct {
	endpoints []Endpoint
	pick      func([]Endpoint, CallOptions) (Endpoint, error)
}

var _ Balancer = &balancerFuncImpl{}

// BalancerFunc returns a stateless, purely functional load balancer that uses
// the provided picking function.
func BalancerFunc(pick func([]Endpoint, CallOptions) (Endpoint, error)) Balancer {
	return &balancerFuncImpl{pick: pick}
}

func (bf *balancerFuncImpl) Update(endpoints []Endpoint) {
	bf.endpoints = endpoints
}

func (bf *balancerFuncImpl) Pick(opts CallOptions) (Endpoint, error) {
	return bf.pick(bf.endpoints, opts)
}

type roundRobin struct {
	endpoints []Endpoint
	next      int
}

var _ Balancer = &roundRobin{}

// RoundRobin returns a round-robin balancer.
func RoundRobin() *roundRobin {
	return &roundRobin{}
}

func (rr *roundRobin) Update(endpoints []Endpoint) {
	rr.endpoints = endpoints
}

func (rr *roundRobin) Pick(CallOptions) (Endpoint, error) {
	if len(rr.endpoints) == 0 {
		return nil, fmt.Errorf("%w: no endpoints available", Unreachable)
	}
	if rr.next >= len(rr.endpoints) {
		rr.next = 0
	}
	endpoint := rr.endpoints[rr.next]
	rr.next += 1
	return endpoint, nil
}

// Sharded returns a new sharded balancer.
//
// Given a list of n endpoints e1, ..., en, for a request with shard key k, a
// sharded balancer will pick endpoint ei where i = k mod n. If no shard key is
// provided, an endpoint is picked at random.
func Sharded() Balancer {
	return BalancerFunc(func(endpoints []Endpoint, opts CallOptions) (Endpoint, error) {
		n := len(endpoints)
		if n == 0 {
			return nil, fmt.Errorf("%w: no endpoints available", Unreachable)
		}
		if opts.ShardKey == 0 {
			// There is no ShardKey. Pick an endpoint at random.
			return endpoints[rand.Intn(n)], nil
		}
		return endpoints[opts.ShardKey%uint64(n)], nil
	})
}
