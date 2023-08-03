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

// ReplicaConnection is a connection to a single replica. A single Connection
// may consist of many ReplicaConnections (typically one per replica).
type ReplicaConnection interface {
	// Address returns the name of the endpoint to which the ReplicaConnection
	// is connected.
	Address() string
}

// Balancer manages a set of ReplicaConnections and picks one of them per
// call. A Balancer requires external synchronization (no concurrent calls
// should be made to the same Balancer).
//
// TODO(mwhittaker): Right now, balancers have no load information about
// endpoints. In the short term, we can at least add information about the
// number of pending requests for every endpoint.
type Balancer interface {
	// Add adds a ReplicaConnection to the set of connections.
	Add(ReplicaConnection)

	// Remove removes a ReplicaConnection from the set of connections.
	Remove(ReplicaConnection)

	// Pick picks a ReplicaConnection from the set of connections.
	// Pick returns _,false if no connections are available.
	Pick(CallOptions) (ReplicaConnection, bool)
}

// balancerFuncImpl is the implementation of the "functional" balancer
// returned by BalancerFunc.
type balancerFuncImpl struct {
	connList
	pick func([]ReplicaConnection, CallOptions) (ReplicaConnection, bool)
}

var _ Balancer = &balancerFuncImpl{}

// BalancerFunc returns a stateless, purely functional load balancer that calls
// pick to pick the connection to use.
func BalancerFunc(pick func([]ReplicaConnection, CallOptions) (ReplicaConnection, bool)) Balancer {
	return &balancerFuncImpl{pick: pick}
}

func (bf *balancerFuncImpl) Pick(opts CallOptions) (ReplicaConnection, bool) {
	return bf.pick(bf.list, opts)
}

type roundRobin struct {
	connList
	next int
}

var _ Balancer = &roundRobin{}

// RoundRobin returns a round-robin balancer.
func RoundRobin() *roundRobin {
	return &roundRobin{}
}

func (rr *roundRobin) Pick(CallOptions) (ReplicaConnection, bool) {
	if len(rr.list) == 0 {
		return nil, false
	}
	if rr.next >= len(rr.list) {
		rr.next = 0
	}
	c := rr.list[rr.next]
	rr.next += 1
	return c, true
}

// connList is a helper type used by balancers to maintain set of connections.
type connList struct {
	list []ReplicaConnection
}

func (cl *connList) Add(c ReplicaConnection) {
	cl.list = append(cl.list, c)
}

func (cl *connList) Remove(c ReplicaConnection) {
	for i, elem := range cl.list {
		if elem != c {
			continue
		}
		// Replace removed entry with last entry.
		cl.list[i] = cl.list[len(cl.list)-1]
		cl.list = cl.list[:len(cl.list)-1]
		return
	}
}
