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
	"context"
	"fmt"
)

// A Resolver manages a potentially changing set of endpoints. For example:
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
//	    var version *Version
//	    for ctx.Err() == nil {
//	        endpoints, newVersion, err = resolver.Resolve(ctx, version)
//	        if err != nil {
//	            return err
//	        }
//	        version = newVersion
//
//	        for _, endpoint := range endpoints {
//	            fmt.Println(endpoint.Address())
//	        }
//
//	        if resolver.IsConstant() {
//	            return nil
//	        }
//	    }
//	    return ctx.Err()
//	}
type Resolver interface {
	// IsConstant returns whether a resolver is constant. A constant resolver
	// returns a fixed set of endpoints that doesn't change over time. A
	// non-constant resolver manages a set of endpoints that does change over
	// time.
	IsConstant() bool

	// Resolve returns a resolver's set of dialable endpoints. For non-constant
	// resolvers, this set of endpoints may change over time. Every snapshot of
	// the set of endpoints is assigned a unique version. If you call the
	// Resolve method with a nil version, Resolve returns the current set of
	// endpoints and its version. If you call the Resolve method with a non-nil
	// version, then a Resolver either:
	//    1. Blocks until the latest set of endpoints has a version newer than
	//       the one provided, returning the new set of endpoints and a new
	//       version.
	//    2. Returns the same version, indicating that the Resolve should
	//       be called again after an appropriate delay.
	//
	// Example:
	//     if !resolver.IsConstant() {
	//         // Perform an unversioned, non-blocking Resolve to get the the
	//         // latest set of endpoints and its version.
	//         endpoints, version, err := resolver.Resolve(ctx, nil)
	//
	//         // Perform a versioned Resolve that either (1) blocks until a set
	//         // of endpoints exists with a version newer than `version`, or
	//         // (2) returns `version`, indicating that the Resolve should be
	//         // called again after an appropriate delay.
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
	Resolve(ctx context.Context, version *Version) ([]Endpoint, *Version, error)
}

// Version is the version associated with a resolver's set of endpoints.
// Versions are opaque entities and should not be inspected or interpreted.
// Versions should only ever be constructed by calling a resolver's Resolve
// method and should only ever be used by being passed to the same resolver's
// Resolve method.
type Version struct {
	Opaque string
}

// Missing is the version associated with a value that does not exist in the
// store.
//
// TODO(rgrandl): this should be the same as the one in gke/internal/store/store.go.
// Should we move the version into a common file? Right now we have a duplicate
// version struct that is used both by the gke/store and stub/resolver.
var Missing = Version{"__tombstone__"}

// constantResolver is a trivial constant resolver that returns a fixed set of
// endpoints.
type constantResolver struct {
	endpoints []Endpoint
}

var _ Resolver = &constantResolver{}

// NewConstantResolver returns a new resolver that returns the provided
// set of endpoints.
func NewConstantResolver(endpoints ...Endpoint) Resolver {
	return &constantResolver{endpoints: endpoints}
}

// IsConstant implements the Resolver interface.
func (*constantResolver) IsConstant() bool {
	return true
}

// Resolve implements the Resolver interface.
func (c *constantResolver) Resolve(_ context.Context, version *Version) ([]Endpoint, *Version, error) {
	if version != nil {
		return nil, nil, fmt.Errorf("unexpected non-nil version %v", *version)
	}
	return c.endpoints, nil, nil
}
