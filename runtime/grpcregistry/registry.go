package grpcregistry

import (
	"context"
	"fmt"
	"net"
	"os"
	"reflect"
	"strings"
	"sync"

	"github.com/ServiceWeaver/weaver/internal/resolver"
	"golang.org/x/exp/maps"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

var globalRegistry registry

type registry struct {
	mu         sync.Mutex
	components map[string]*Component
}

// Registration this is the same as the one in the weaver package that the user has to write.
type Registration struct {
	ClientType reflect.Type
	Client     func(grpc.ClientConnInterface) any
	Server     func(s grpc.ServiceRegistrar) error
}

func Register(reg Registration) {
	if err := globalRegistry.register(reg); err != nil {
		panic(err)
	}
}

type Component struct {
	Name   string
	Handle *Handle // contains info about clients to connect to the grpc server
	Impl   func(ctx context.Context, lis net.Listener) error
}

type Handle struct {
	Client   any
	Conn     *grpc.ClientConn   // the underlying grpc connection.
	Resolver *resolver.Resolver // responsible to update the endpoints.
}

func (r *registry) register(reg Registration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.components == nil {
		r.components = map[string]*Component{}
	}

	// Build client implementation.
	name, err := getServiceName(reg.ClientType.Name())
	if err != nil {
		return err
	}
	resolver := &resolver.Resolver{Name: strings.ToLower(name)}
	conn, err := grpc.Dial(resolver.Scheme()+":///", grpc.WithResolvers(resolver), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	client := reg.Client(conn)
	if _, found := r.components[name]; found {
		return fmt.Errorf("component %s already registered", name)
	}

	// Build server implementation.
	implFn := func(ctx context.Context, lis net.Listener) error {
		fmt.Fprintf(os.Stderr, "[%s][%d] Server listening on %v\n", name, os.Getpid(), lis.Addr())
		s := grpc.NewServer()
		reflection.Register(s)
		if err := reg.Server(s); err != nil {
			return err
		}
		return s.Serve(lis)
	}

	r.components[name] = &Component{
		Name:   name,
		Handle: &Handle{Client: client, Conn: conn, Resolver: resolver},
		Impl:   implFn,
	}
	return nil
}

// Registered returns all groups registered by the user.
func Registered() []*Component {
	return globalRegistry.allComponents()
}

// GetGrpcClient returns the grpc client corresponding to a grpc server that has registered service t.
func GetGrpcClient(t reflect.Type) (any, error) {
	return globalRegistry.getGrpcClient(t)
}

func (r *registry) allComponents() []*Component {
	r.mu.Lock()
	defer r.mu.Unlock()
	return maps.Values(r.components)
}

func (r *registry) getGrpcClient(t reflect.Type) (any, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name, err := getServiceName(t.Name())
	if err != nil {
		return nil, err
	}
	c, ok := r.components[name]
	if !ok {
		return nil, fmt.Errorf("unable to find registered client for service: %s", name)
	}
	return c.Handle.Client, nil
}

func getServiceName(s string) (string, error) {
	suffix := "Client"
	if strings.HasSuffix(s, suffix) {
		return s[:len(s)-len(suffix)], nil
	}
	return "", fmt.Errorf("unable to extract service name from: %s", s)
}
