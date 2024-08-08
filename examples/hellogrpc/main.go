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

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/ServiceWeaver/weaver"
	"google.golang.org/grpc"
)

func main() {
	weaver.RegisterComponent(
		NewHelloFromMTVClient,
		func(s grpc.ServiceRegistrar) error {
			RegisterHelloFromMTVServer(s, &HelloGrpcMTV{})
			return nil
		},
	)
	weaver.RegisterComponent(
		NewHelloFromSVLClient,
		func(s grpc.ServiceRegistrar) error {
			h, err := weaver.GetClient[HelloFromMTVClient]()
			if err != nil {
				return err
			}
			RegisterHelloFromSVLServer(s, &HelloGrpcSVL{mtvHandle: h})
			return nil
		},
	)

	if err := weaver.RunGrpc(context.Background(), run); err != nil {
		panic(err)
	}
}

func run(context.Context) error {
	lis, err := net.Listen("tcp", "127.0.0.1:60000")
	if err != nil {
		// This listener can eventually be a weaver listener.
		return err
	}

	clientMTV, err := weaver.GetClient[HelloFromMTVClient]()
	if err != nil {
		return err
	}
	clientSVL, err := weaver.GetClient[HelloFromSVLClient]()
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
		res := fmt.Sprintf("%s\n%s\n", r2.Response, r1.Response)
		fmt.Fprintf(w, res)
	})
	return http.Serve(lis, nil)
}
