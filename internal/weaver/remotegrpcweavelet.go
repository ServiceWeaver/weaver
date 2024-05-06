package weaver

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/ServiceWeaver/weaver/internal/control"
	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/grpcregistry"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/retry"
	"github.com/ServiceWeaver/weaver/runtime/version"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"
)

type RemoteWeaveletGrpc struct {
	control.WeaveletControlGrpcServer

	ctx        context.Context
	servers    *errgroup.Group
	args       *protos.WeaveletArgs
	weaverInfo *WeaverInfo
	deployer   control.DeployerControlGrpcClient // component to control deployer

	// state to synchronize with envelope initiated initialization handshake.
	initMu     sync.Mutex
	initCalled bool
	initDone   chan struct{}

	// channel that is closed when deployer is ready.
	deployerReady chan struct{}

	groups map[string]*group // groups that we should start each of them in a separate process

	lis net.Listener // Address dialed by other groups
}

type group struct {
	reg *grpcregistry.Group

	activateInit sync.Once // used to activate the group
	activateErr  error     // non-nil if activation fails

	implInit sync.Once // used to initialize impl
	implErr  error     // non-nil if impl creation fails

	replicas []string
}

func NewRemoteWeaveletGrpc(ctx context.Context, groups []*grpcregistry.Group,
	bootstrap runtime.Bootstrap) (*RemoteWeaveletGrpc, error) {

	// Get the arguments.
	args := bootstrap.Args

	// Make internal listener.
	lis, err := net.Listen("tcp", args.InternalAddress)
	if err != nil {
		return nil, err
	}

	servers, ctx := errgroup.WithContext(ctx)
	w := &RemoteWeaveletGrpc{
		ctx:           ctx,
		servers:       servers,
		groups:        map[string]*group{},
		args:          args,
		weaverInfo:    &WeaverInfo{DeploymentID: args.DeploymentId},
		initDone:      make(chan struct{}),
		deployerReady: make(chan struct{}),
		lis:           lis,
	}

	// Initialize the registered groups.
	for _, g := range groups {
		w.groups[g.Name] = &group{reg: g}
	}

	info := bootstrap.Args

	// Serve the control component.
	controlAddr, err := net.Listen("tcp", info.ControlSocket)
	if err != nil {
		return nil, err
	}

	servers.Go(func() error {
		s := grpc.NewServer()
		defer s.GracefulStop()
		control.RegisterWeaveletControlGrpcServer(s, w)
		if err := s.Serve(controlAddr); err != nil {
			return err
		}
		return nil
	})

	// Get a handle to the deployer control component.
	w.deployer, err = w.getDeployerControl()
	if err != nil {
		return nil, err
	}
	close(w.deployerReady)

	// Wait for initialization handshake to complete, so we have full config info.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-w.initDone:
		// Ready to serve
	}

	// Try to start all the groups.
	for gname, g := range w.groups {
		g.activateInit.Do(func() {
			errMsg := fmt.Sprintf("cannot activate group %q", gname)
			g.activateErr = w.repeatedly(w.ctx, errMsg, func() error {
				request := &protos.ActivateComponentRequest{
					Component: gname,
				}
				_, err := w.deployer.ActivateComponent(w.ctx, request)
				return err
			})
		})
		if g.activateErr != nil {
			return nil, g.activateErr
		}
	}
	fmt.Fprintf(os.Stderr, "🧶 weavelet started %s\n", w.lis.Addr())
	return w, nil
}

// Wait waits for the RemoteWeavelet to fully shut down after its context has
// been cancelled.
func (w *RemoteWeaveletGrpc) Wait() error {
	return w.servers.Wait()
}

func (w *RemoteWeaveletGrpc) Info() *protos.WeaveletArgs {
	return w.args
}

func (r *RemoteWeaveletGrpc) getDeployerControl() (control.DeployerControlGrpcClient, error) {
	conn, err := grpc.Dial(r.args.DeployerSocket, grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	// Robert - close connection somehow!? conn.Close()
	return control.NewDeployerControlGrpcClient(conn), nil
}

func (r *RemoteWeaveletGrpc) InitWeavelet(context.Context, *protos.InitWeaveletRequest) (*protos.InitWeaveletReply, error) {
	r.initMu.Lock()
	defer r.initMu.Unlock()
	if !r.initCalled {
		r.initCalled = true
		close(r.initDone)
	}

	return &protos.InitWeaveletReply{
		DialAddr: r.lis.Addr().String(),
		Version: &protos.SemVer{
			Major: version.DeployerMajor,
			Minor: version.DeployerMinor,
			Patch: 0,
		},
	}, nil
}

func (r *RemoteWeaveletGrpc) UpdateComponents(_ context.Context, req *protos.UpdateComponentsRequest) (*protos.UpdateComponentsReply, error) {
	for _, g := range req.Components {
		go func() {
			gr, ok := r.groups[g]
			if !ok {
				return
			}
			gr.implInit.Do(func() {
				gr.implErr = gr.reg.Start(r.ctx, r.lis)
			})
		}()
	}
	return &protos.UpdateComponentsReply{}, nil
}

func (r *RemoteWeaveletGrpc) UpdateRoutingInfo(_ context.Context, req *protos.UpdateRoutingInfoRequest) (*protos.UpdateRoutingInfoReply, error) {
	if req.RoutingInfo == nil {
		return nil, fmt.Errorf("nil RoutingInfo")
	}
	info := req.RoutingInfo
	group, ok := r.groups[req.RoutingInfo.Component]
	if !ok {
		panic(fmt.Errorf("unable to find group: %s\n", req.RoutingInfo.Component))
	}

	for _, replica := range info.Replicas {
		if !slices.Contains(group.replicas, replica) {
			group.replicas = append(group.replicas, replica)
		}
	}
	endpoints := []resolver.Endpoint{}
	for _, addr := range group.replicas {
		endpoints = append(endpoints, resolver.Endpoint{
			Addresses: []resolver.Address{{Addr: addr}},
		})
	}

	var mu sync.Mutex
	mu.Lock()
	defer mu.Unlock()

	group.reg.Accessor.Resolver.UpdateState(resolver.State{
		Endpoints: endpoints,
	})

	return &protos.UpdateRoutingInfoReply{}, nil
}

func (r *RemoteWeaveletGrpc) GetHealth(ctx context.Context, req *protos.GetHealthRequest) (*protos.GetHealthReply, error) {
	panic("GetHealth not implemented")
}

func (r *RemoteWeaveletGrpc) GetLoad(ctx context.Context, req *protos.GetLoadRequest) (*protos.GetLoadReply, error) {
	panic("GetLoad not implemented")
}

func (r *RemoteWeaveletGrpc) GetMetrics(ctx context.Context, req *protos.GetMetricsRequest) (*protos.GetMetricsReply, error) {
	panic("GetMetrics not implemented")
}

func (r *RemoteWeaveletGrpc) GetProfile(ctx context.Context, req *protos.GetProfileRequest) (*protos.GetProfileReply, error) {
	panic("GetProfile not implemented")
}

func (w *RemoteWeaveletGrpc) repeatedly(ctx context.Context, errMsg string, f func() error) error {
	for r := retry.Begin(); r.Continue(ctx); {
		if err := f(); err != nil {
			fmt.Fprintf(os.Stderr, fmt.Sprintf("%s; will retry: %v", errMsg, err))
			continue
		}
		return nil
	}
	return fmt.Errorf("%s: %w", errMsg, ctx.Err())
}
