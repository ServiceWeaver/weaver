package resolver

import (
	"fmt"
	"os"
	"sync"

	"google.golang.org/grpc/resolver"
)

type Resolver struct {
	Name string

	// Fields actually belong to the resolver.
	// Guards access to below fields.
	mu sync.Mutex
	CC resolver.ClientConn
	// Storing the most recent state update makes this resolver resilient to
	// restarts, which is possible with channel idleness.
	lastSeenState *resolver.State
}

func (r *Resolver) Scheme() string {
	return r.Name
}

func (r *Resolver) UpdateState(s resolver.State) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.CC == nil {
		panic("cannot update state as grpc.Dial with resolver has not been called")
	}

	if err := r.CC.UpdateState(s); err != nil {
		fmt.Fprintf(os.Stderr, "Update State for group %s with err: %v\n", r.Name, err)
	}
	r.lastSeenState = &s
}

func (r *Resolver) ResolveNow(resolver.ResolveNowOptions) {
	// noop for resolver
}

func (r *Resolver) Close() {
	// noop for resolver
}

func (r *Resolver) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.CC = cc
	var err error
	if r.lastSeenState != nil {
		err = r.CC.UpdateState(*r.lastSeenState)
	}
	return r, err
}
