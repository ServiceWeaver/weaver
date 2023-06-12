// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package simple is used for a trivial test of code that uses Service Weaver.
//
// We have two components Source and Destination and ensure that a call to Source
// gets reflected in a call to Destination.
package simple

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/ServiceWeaver/weaver"
)

//go:generate ../../../cmd/weaver/weaver generate

type Source interface {
	Emit(ctx context.Context, file, msg string) error
}

type source struct {
	weaver.Implements[Source]
	dst weaver.Ref[Destination]
}

func (s *source) Emit(ctx context.Context, file, msg string) error {
	return s.dst.Get().Record(ctx, file, msg)
}

type Destination interface {
	Getpid(_ context.Context) (int, error)
	Record(_ context.Context, file, msg string) error
	GetAll(_ context.Context, file string) ([]string, error)
	RoutedRecord(_ context.Context, file, msg string) error
}

type destRouter struct{}

func (r destRouter) RoutedRecord(_ context.Context, file, msg string) string {
	return file
}

type destination struct {
	weaver.Implements[Destination]
	weaver.WithRouter[destRouter]
	mu sync.Mutex
}

var pid = os.Getpid()

func (d *destination) Getpid(_ context.Context) (int, error) {
	return pid, nil
}

// Record adds a message.
func (d *destination) Record(_ context.Context, file, msg string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(msg + "\n")
	return err
}

func (d *destination) RoutedRecord(ctx context.Context, file, msg string) error {
	return d.Record(ctx, file, "routed: "+msg)
}

// GetAll returns all added messages.
func (d *destination) GetAll(_ context.Context, file string) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	str := strings.TrimSpace(string(data))
	return strings.Split(str, "\n"), nil
}

// Server is a component used to test Service Weaver listener handling.
// An HTTP server is started when this component is initialized.
// simple_test.go checks the functionality of the HTTP server by fetching
// from a well-known URL on the server.
type Server interface {
	Address(context.Context) (string, error)
	ProxyAddress(context.Context) (string, error)
	Shutdown(context.Context) error
}

const ServerTestResponse = "hello world"

type server struct {
	weaver.Implements[Server]
	addr  string
	proxy string
	hello weaver.Listener
	srv   *http.Server
}

func (s *server) Init(ctx context.Context) error {
	//nolint:nolintlint,typecheck // golangci-lint false positive on Go tip
	s.addr = s.hello.String()
	s.proxy = s.hello.ProxyAddr()

	// Run server on listener.
	s.srv = &http.Server{
		Handler: weaver.InstrumentHandlerFunc("test", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, ServerTestResponse)
		}),
	}
	go func() {
		err := s.srv.Serve(s.hello)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}()
	return nil
}

func (s *server) Address(ctx context.Context) (string, error)      { return s.addr, nil }
func (s *server) ProxyAddress(ctx context.Context) (string, error) { return s.proxy, nil }
func (s *server) Shutdown(ctx context.Context) error               { return s.srv.Shutdown(ctx) }
