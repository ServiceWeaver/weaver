package grpcregistry

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/ServiceWeaver/weaver/internal/resolver"
	"golang.org/x/exp/maps"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

var globalRegistry registry

// This is the same as the one in the weaver package that the user has to write.
type Registration struct {
	Name     string
	Clients  func(cc grpc.ClientConnInterface) []any
	Services func(s grpc.ServiceRegistrar)
	// Robert: listeners?
}

func Register(reg *Registration) error {
	return globalRegistry.register(reg)
}

// Registered returns all groups registered by the user.
func Registered() []*Group {
	return globalRegistry.Groups()
}

// GetGrpcClient returns the grpc client corresponding to a grpc server that has registered service t.
func GetGrpcClient(ctx context.Context, t reflect.Type) (any, error) {
	return globalRegistry.getGrpcClient(ctx, t)
}

type registry struct {
	mu     sync.Mutex
	groups map[string]*Group
}

type Group struct {
	Name string

	// Implementations of the registrations.
	Accessor *Accessor // contains info about clients to connect to the grpc server
	ImplFn   func(ctx context.Context, lis net.Listener) error
}

type Accessor struct {
	Clients  map[reflect.Type]any
	Conn     *grpc.ClientConn   // the underlying shared connection between clients.
	Resolver *resolver.Resolver // responsible to update the endpoints
}

func (g *Group) Start(ctx context.Context, lis net.Listener) error {
	return g.ImplFn(ctx, lis)
}

func (r *registry) Groups() []*Group {
	r.mu.Lock()
	defer r.mu.Unlock()
	return maps.Values(r.groups)
}

func (r *registry) register(reg *Registration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.groups[reg.Name]; ok {
		return fmt.Errorf("group %s already registered", reg.Name)
	}
	if r.groups == nil {
		r.groups = map[string]*Group{}
	}

	// Build server implementation.
	implFn := func(ctx context.Context, lis net.Listener) error {
		fmt.Fprintf(os.Stderr, "[%s] Server listening on %v\n", reg.Name, lis.Addr())
		s := grpc.NewServer()
		reflection.Register(s)
		reg.Services(s) // register all services required by the client for this registration.
		errs := make(chan error, 1)
		go func() { errs <- s.Serve(lis) }()
		select {
		case err := <-errs:
			return err
		case <-ctx.Done():
			s.GracefulStop()
			return nil
		}
	}

	// Build client implementation.
	resolver := &resolver.Resolver{Name: strings.ToLower(reg.Name) + strconv.Itoa(rand.IntN(200))}
	conn, err := grpc.Dial(
		resolver.Scheme()+":///",
		grpc.WithResolvers(resolver),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	clients := map[reflect.Type]any{}
	for _, a := range reg.Clients(conn) {
		clients[reflect.TypeOf(a).Elem()] = a
	}

	r.groups[reg.Name] = &Group{
		Name:     reg.Name,
		Accessor: &Accessor{Conn: conn, Resolver: resolver, Clients: clients},
		ImplFn:   implFn,
	}
	return nil
}

func (r *registry) getGrpcClient(_ context.Context, t reflect.Type) (any, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, g := range r.groups {
		for cltype, client := range g.Accessor.Clients {
			if strings.ToLower(t.Name()) == strings.ToLower(cltype.Name()) {
				return client, nil
			}
		}
	}
	return nil, fmt.Errorf("unable to find grpc client for %s", t.Name())
}
