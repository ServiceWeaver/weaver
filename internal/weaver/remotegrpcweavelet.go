// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package weaver

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/ServiceWeaver/weaver/internal/controlgrpc"
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
	controlgrpc.WeaveletControlGrpcServer

	args       *protos.WeaveletArgs
	weaverInfo *WeaverInfo

	ctx     context.Context
	servers *errgroup.Group

	deployer controlgrpc.DeployerControlGrpcClient // component to control deployer

	// state to synchronize with envelope initiated initialization handshake.
	initMu     sync.Mutex
	initCalled bool
	initDone   chan struct{}

	groups map[string]*group // groups that we should start each of them in a separate process

	internalLis net.Listener // Address dialed by other groups
}

type group struct { // a group runs a single component for now
	component *grpcregistry.Component

	activateInit sync.Once // used to activate the group
	activateErr  error     // non-nil if activation fails

	implInit sync.Once // used to initialize impl
	implErr  error     // non-nil if impl creation fails

	replicas []string
}

func NewRemoteWeaveletGrpc(ctx context.Context, components []*grpcregistry.Component, bootstrap runtime.Bootstrap) (*RemoteWeaveletGrpc, error) {
	servers, ctx := errgroup.WithContext(ctx)

	// Get the arguments.
	args := bootstrap.Args

	// Make internal listener.
	lis, err := net.Listen("tcp", args.InternalAddress)
	if err != nil {
		return nil, err
	}

	w := &RemoteWeaveletGrpc{
		ctx:         ctx,
		servers:     servers,
		groups:      map[string]*group{},
		args:        args,
		weaverInfo:  &WeaverInfo{DeploymentID: args.DeploymentId},
		initDone:    make(chan struct{}),
		internalLis: lis,
	}

	// Initialize the groups, one for each registered component.
	for _, c := range components {
		w.groups[c.Name] = &group{component: c}
	}

	// Serve the control component.
	controlAddr, err := net.Listen("tcp", args.ControlSocket)
	if err != nil {
		return nil, err
	}

	servers.Go(func() error {
		s := grpc.NewServer()
		defer s.GracefulStop()
		controlgrpc.RegisterWeaveletControlGrpcServer(s, w)
		return s.Serve(controlAddr)
	})

	// Get a handle to the deployer control component.
	w.deployer, err = w.getDeployerControl()
	if err != nil {
		return nil, err
	}

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

	fmt.Fprintf(os.Stderr, "[%d] 🧶 weavelet started %s\n", os.Getpid(), w.internalLis.Addr())
	return w, nil
}

func (r *RemoteWeaveletGrpc) getDeployerControl() (controlgrpc.DeployerControlGrpcClient, error) {
	conn, err := grpc.Dial(r.args.DeployerSocket, grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	// Robert - close connection somehow!? conn.Close()
	return controlgrpc.NewDeployerControlGrpcClient(conn), nil
}

// Wait waits for the RemoteWeavelet to fully shut down after its context has
// been cancelled.
func (w *RemoteWeaveletGrpc) Wait() error {
	return w.servers.Wait()
}

func (w *RemoteWeaveletGrpc) Info() *protos.WeaveletArgs {
	return w.args
}

func (r *RemoteWeaveletGrpc) InitWeavelet(context.Context, *protos.InitWeaveletRequest) (*protos.InitWeaveletReply, error) {
	r.initMu.Lock()
	defer r.initMu.Unlock()
	if !r.initCalled {
		r.initCalled = true
		close(r.initDone)
	}

	return &protos.InitWeaveletReply{
		DialAddr: r.internalLis.Addr().String(),
		Version: &protos.SemVer{
			Major: version.DeployerMajor,
			Minor: version.DeployerMinor,
			Patch: 0,
		},
	}, nil
}

func (r *RemoteWeaveletGrpc) UpdateComponents(_ context.Context, req *protos.UpdateComponentsRequest) (*protos.UpdateComponentsReply, error) {
	for _, g := range req.Components {
		go func(g string) {
			gr, ok := r.groups[g]
			if !ok {
				return
			}
			gr.implInit.Do(func() {
				gr.implErr = gr.component.Impl(r.ctx, r.internalLis)
			})
		}(g)
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

	group.component.Handle.Resolver.UpdateState(resolver.State{
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
