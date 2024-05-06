package main

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/ServiceWeaver/weaver"
	"google.golang.org/grpc"
)

// This should have 3 groups: main, HelloGrpcSVL and HelloGrpcMTV.
func main() {
	registrations := []*weaver.Registration{
		{
			Name: "HelloGrpcSVL",
			Clients: func(cc grpc.ClientConnInterface) []any {
				return []any{NewHelloFromSVLClient(cc)}
			},
			Services: func(s grpc.ServiceRegistrar) {
				RegisterHelloFromSVLServer(s, &HelloGrpcSVL{})
			},
		},
		{
			Name: "HelloGrpcMTV",
			Clients: func(cc grpc.ClientConnInterface) []any {
				return []any{NewHelloFromMTVClient(cc)}
			},
			Services: func(s grpc.ServiceRegistrar) {
				RegisterHelloFromMTVServer(s, &HelloGrpcMTV{})
			},
		},
	}
	if err := weaver.RunGrpc(context.Background(), registrations, bodyToRun); err != nil {
		panic(err)
	}
}
func bodyToRun(ctx context.Context) error {
	lis, err := net.Listen("tcp", ":60000")
	if err != nil {
		// This listener can eventually be a weaver listener.
		return err
	}

	clientMTV, err := weaver.GetClient(ctx, (*HelloFromMTVClient)(nil))
	if err != nil {
		return err
	}
	clientSVL, err := weaver.GetClient(ctx, (*HelloFromSVLClient)(nil))
	if err != nil {
		return err
	}

	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("n")
		r1, err := clientSVL.Hello(r.Context(), &HelloRequest{Request: name})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		r2, err := clientMTV.Hello(r.Context(), &HelloRequest{Request: name})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		res := fmt.Sprintf("%s\n%s\n", r1.Response, r2.Response)
		fmt.Fprintf(w, res)
	})
	return http.Serve(lis, nil)
}
