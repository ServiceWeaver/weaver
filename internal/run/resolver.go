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
	"net"
	"slices"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// A Resolver returns a potentially changing set of addresses. For example:
//
//   - A DNS resolver might resolve a hostname like "google.com" into a set of
//     IP addresses like ["74.125.142.106", "74.125.142.103", "74.125.142.99"].
//   - A Kubernetes Service resolver might resolve a service name like
//     "backend" into the IP addresses of the pods that implement the Service.
//
// # Example
//
//	func printAddrs(ctx context.Context, resolver Resolver) error {
//	    for {
//	        select {
//	        case <-ctx.Done():
//	            return ctx.Err()
//
//	        case addrs, ok := <-resolver.Addresses():
//	            if !ok {
//	                return resolver.Err()
//	            }
//	            fmt.Println(addrs)
//	        }
//	    }
//	}
type Resolver interface {
	// Addresses returns a channel of resolved addresses.
	Addresses() <-chan []string

	// Err should be called after the channel returned by Addresses is closed.
	// If the channel was closed because an error was encountered, the error is
	// returned by Err.
	Err() error
}

// resolver is a low-level resolver used by functions like ConstantResolver,
// DNSResolver, CombinedResolver, etc.
type resolver struct {
	addresses chan []string
	mu        sync.Mutex
	err       error
}

// close closes the resolver with the provided error.
func (r *resolver) close(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
	close(r.addresses)
}

// Addresses implements the Resolver interface.
func (r *resolver) Addresses() <-chan []string {
	return r.addresses
}

// Err implements the Resolver interface.
func (r *resolver) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

// ConstantResolver returns a Resolver with the provided set of addresses.
// ConstantResolver panics if no addresses are provided.
func ConstantResolver(addresses ...string) Resolver {
	if len(addresses) == 0 {
		panic(fmt.Errorf("ConstantResolver: no addresses"))
	}
	c := make(chan []string, 1)
	c <- addresses
	close(c)
	return &resolver{addresses: c}
}

// DNSResolver returns a Resolver that periodically resolves the provided
// address using DNS. The provided address must be formatted as "host:port".
// The returned Resolver is shut down and its channel is closed when the
// provided context is cancelled.
func DNSResolver(ctx context.Context, address string) (Resolver, error) {
	// Split the "host:port" addess into host and port.
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("DNSResolver: invalid address %q: %w", address, err)
	}

	r := &resolver{addresses: make(chan []string, 100)}
	var previousIPs []string
	resolve := func() error {
		// Resolve the host into a set of IP addresses.
		ips, err := net.DefaultResolver.LookupHost(ctx, host)
		if err != nil {
			return err
		}

		// Check if the IP addresses have changed.
		sort.Strings(ips)
		if slices.Equal(ips, previousIPs) {
			return nil
		}
		previousIPs = ips

		// Append the port to the returned IP addresses.
		for i, ip := range ips {
			ips[i] = net.JoinHostPort(ip, port)
		}
		r.addresses <- ips
		return nil
	}
	if err := resolve(); err != nil {
		return nil, err
	}

	// Periodically resolve the host.
	var group errgroup.Group
	group.Go(func() error {
		ticker := time.NewTicker(5 * time.Second)
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return ctx.Err()
			case <-ticker.C:
				if err := resolve(); err != nil {
					return err
				}
			}
		}
	})

	// Close the returned channel when resolution fails.
	go func() {
		r.close(group.Wait())
	}()

	return r, nil
}

// CombinedResolver returns returns a Resolver that combines the addresses of
// the provided Resolvers. The returned Resolver is shut down and its channel
// is closed when the provided context is cancelled.
//
// CombinedResolver panics if no Resolvers are provided.
func CombinedResolver(ctx context.Context, resolvers ...Resolver) Resolver {
	if len(resolvers) == 0 {
		panic(fmt.Errorf("CombinedResolver: no resolvers"))
	}

	var mu sync.Mutex                                   // guards the following
	resolved := make([][]string, len(resolvers))        // resolved addresses, one per resolver
	r := &resolver{addresses: make(chan []string, 100)} // combined addresses

	// Launch one watching goroutine for every resolver.
	group, ctx := errgroup.WithContext(ctx)
	for i, resolver := range resolvers {
		i := i
		resolver := resolver
		group.Go(func() error {
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()

				case addresses, ok := <-resolver.Addresses():
					if !ok {
						return resolver.Err()
					}
					mu.Lock()
					resolved[i] = addresses
					r.addresses <- flatten(resolved)
					mu.Unlock()
				}
			}
		})
	}

	// Close the returned channel when (1) the context is cancelled, (2) all
	// resolvers are done resolving, or (3) a resolver errored out.
	go func() {
		r.close(group.Wait())
	}()

	return r
}

// flatten concatenates a slice of slices.
func flatten[T any](xs [][]T) []T {
	var flattened []T
	for _, x := range xs {
		flattened = append(flattened, x...)
	}
	return flattened
}
