package weaver

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/ServiceWeaver/weaver/runtime/grpcregistry"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/resolver"
)

type SingleGrpcWeavelet struct {
	ctx        context.Context
	components []*grpcregistry.Component
	servers    *errgroup.Group
}

func NewSingleGrpcWeavelet(ctx context.Context, components []*grpcregistry.Component) (*SingleGrpcWeavelet, error) {
	servers, ctx := errgroup.WithContext(ctx)

	w := &SingleGrpcWeavelet{
		ctx:        ctx,
		components: components,
		servers:    servers,
	}

	for _, c := range w.components {
		servers.Go(func() error {
			lis, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				return err
			}
			c.Handle.Resolver.UpdateState(resolver.State{
				Endpoints: []resolver.Endpoint{
					{Addresses: []resolver.Address{{Addr: lis.Addr().String()}}},
				},
			})
			return c.Impl(ctx, lis)
		})
	}
	fmt.Fprintf(os.Stderr, "🧶 weavelet started pid - %d\n", os.Getpid())
	return w, nil
}

func (w *SingleGrpcWeavelet) Wait() error {
	return w.servers.Wait()
}
