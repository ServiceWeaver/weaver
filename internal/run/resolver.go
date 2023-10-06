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
	"context"
	"fmt"
)

// A Resolver manages a potentially changing set of addresses. For example:
//
//   - A DNS resolver might resolve a hostname like "google.com" into a set of
//     IP addresses like ["74.125.142.106", "74.125.142.103", "74.125.142.99"].
//   - A Kubernetes Service resolver might resolve a service name like
//     "backend" into the IP addresses of the pods that implement the service.
//   - A unix resolver might resolve a directory name like "/tmp/workers" into
//     the set of unix socket files within the directory.
//
// A Resolver can safely be used concurrently from multiple goroutines.
//
// Example usage:
//
//	func printAddrs(ctx context.Context, resolver Resolver) error {
//	    if resolver.IsConstant() {
//	        addrs, _, err := resolver.Resolve(ctx, nil)
//	        if err != nil {
//	            return err
//	        }
//	        fmt.Println(addrs), nil
//	    }
//
//	    var version *Version
//	    for ctx.Err() == nil {
//	        addrs, newVersion, err := resolver.Resolve(ctx, version)
//	        if err != nil {
//	            return err
//	        }
//	        fmt.Println(addrs)
//	        version = newVersion
//	    }
//	    return ctx.Err()
//	}
type Resolver interface {
	// IsConstant returns whether a resolver is constant. A constant resolver
	// returns a fixed set of addresses that doesn't change over time. A
	// non-constant resolver manages a set of addresses that does change over
	// time.
	IsConstant() bool

	// Resolve returns a resolver's set of dialable addresses. For non-constant
	// resolvers, this set of addresses may change over time. Every snapshot of
	// the set of addresses is assigned a unique version. If you call the
	// Resolve method with a nil version, Resolve returns the current set of
	// addresses and its version. If you call the Resolve method with a non-nil
	// version, then a Resolver blocks until the latest set of addresses has a
	// version newer than the one provided, returning the new set of endpoints
	// and a new version.
	//
	// # Example
	//
	//     if !resolver.IsConstant() {
	//         // Perform an unversioned, non-blocking Resolve to get the the
	//         // latest set of endpoints and its version.
	//         endpoints, version, err := resolver.Resolve(ctx, nil)
	//
	//         // Perform a versioned Resolve that either blocks until a set
	//         // of endpoints exists with a version newer than `version`.
	//         newEndpoints, newVersion, err := resolver.Resolve(ctx, version)
	//     }
	//
	// If the resolver is constant, then Resolve only needs to be called once
	// with a nil version. The returned set of endpoints will never change, and
	// the returned version is nil.
	//
	//     if resolver.IsConstant() {
	//         // endpoints1 == endpoints2, and version1 == version2 == nil.
	//         endpoints1, version1, err := resolver.Resolve(ctx, nil)
	//         endpoints2, version2, err := resolver.Resolve(ctx, nil)
	//     }
	Resolve(ctx context.Context, version *Version) ([]string, *Version, error)
}

// Version is the version associated with a resolver's set of addresses.
// Versions are opaque entities and should not be inspected or interpreted.
// Versions should only ever be constructed by calling a resolver's Resolve
// method and should only ever be used by being passed to the same resolver's
// Resolve method.
type Version struct {
	Opaque string
}

// constantResolver is a trivial constant resolver that returns a fixed set of
// endpoints.
type constantResolver struct {
	addresses []string
}

var _ Resolver = &constantResolver{}

// NewConstantResolver returns a new resolver that returns the provided
// set of addresses.
func NewConstantResolver(addresses ...string) Resolver {
	return &constantResolver{addresses: addresses}
}

// IsConstant implements the Resolver interface.
func (*constantResolver) IsConstant() bool {
	return true
}

// Resolve implements the Resolver interface.
func (c *constantResolver) Resolve(_ context.Context, version *Version) ([]string, *Version, error) {
	if version != nil {
		return nil, nil, fmt.Errorf("unexpected non-nil version %v", *version)
	}
	return c.addresses, nil, nil
}
