package weaver

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/ServiceWeaver/weaver/runtime/grpcregistry"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/resolver"
)

type SingleGrpcWeavelet struct {
	ctx    context.Context
	groups []*grpcregistry.Group // registered groups

	servers *errgroup.Group

	mu      sync.Mutex
	started map[string]bool
}

func NewSingleGrpcWeavelet(ctx context.Context, groups []*grpcregistry.Group) (*SingleGrpcWeavelet, error) {
	servers, ctx := errgroup.WithContext(ctx)

	w := &SingleGrpcWeavelet{
		ctx:     ctx,
		groups:  groups,
		started: map[string]bool{},
		servers: servers,
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	for _, g := range w.groups {
		servers.Go(func() error {
			g.Accessor.Resolver.UpdateState(resolver.State{
				Endpoints: []resolver.Endpoint{
					{Addresses: []resolver.Address{{Addr: lis.Addr().String()}}},
				},
			})
			return g.Start(ctx, lis)
		})
	}

	fmt.Fprintf(os.Stderr, "🧶 weavelet started %s: pid - %d\n", lis.Addr(), os.Getpid())
	return w, nil
}

func (w *SingleGrpcWeavelet) Wait() error {
	return w.servers.Wait()
}
