package weaver

import (
	"context"
	"reflect"

	"github.com/ServiceWeaver/weaver/internal/weaver"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/grpcregistry"
	"google.golang.org/grpc"
)

// Registration as passed by the user code.
type Registration struct {
	Name     string
	Clients  func(cc grpc.ClientConnInterface) []any
	Services func(s grpc.ServiceRegistrar)
	// Robert: listeners?
}

// GetClient - returns a grpc client corresponding to a grpc server that run in
// a colocation group. The grpc client should load balance requests across all
// the server replicas.
func GetClient[T any](ctx context.Context, p *T) (T, error) {
	h, err := grpcregistry.GetGrpcClient(ctx, reflect.TypeOf(p).Elem())
	return h.(T), err
}

// RunGrpc - similar to weaver.Run, runs an app.
func RunGrpc(ctx context.Context, regs []*Registration, app func(ctx context.Context) error) error {
	// Register the groups.
	for _, r := range regs {
		if err := grpcregistry.Register(&grpcregistry.Registration{
			Name:     r.Name,
			Clients:  r.Clients,
			Services: r.Services,
		}); err != nil {
			return err
		}
	}

	// Decides whether to run locally or remote.
	bootstrap, err := runtime.GetBootstrap(ctx)
	if err != nil {
		return err
	}
	if !bootstrap.Exists() {
		// Run the app in a single process.
		return runGrpcLocal(ctx, app)
	}
	// Otherwise, run the local distributed.
	return runGrpcRemote(ctx, app, bootstrap)
}

func runGrpcLocal(ctx context.Context, app func(ctx context.Context) error) error {
	regs := grpcregistry.Registered()

	wlet, err := weaver.NewSingleGrpcWeavelet(ctx, regs)
	if err != nil {
		return err
	}

	// Return when either (1) the remote weavelet exits, or (2) the user provided
	// app function returns, whichever happens first.
	errs := make(chan error, 2)
	go func() {
		errs <- app(ctx)
	}()
	go func() {
		errs <- wlet.Wait()
	}()
	return <-errs
}

func runGrpcRemote(ctx context.Context, app func(ctx context.Context) error, bootstrap runtime.Bootstrap) error {
	regs := grpcregistry.Registered()

	wlet, err := weaver.NewRemoteWeaveletGrpc(ctx, regs, bootstrap)
	if err != nil {
		return err
	}

	// Return when either (1) the remote weavelet exits, or (2) the user provided
	// app function returns, whichever happens first.
	errs := make(chan error, 2)
	if wlet.Info().RunMain {
		go func() {
			errs <- app(ctx)
		}()
	}
	go func() {
		errs <- wlet.Wait()
	}()
	return <-errs
}
