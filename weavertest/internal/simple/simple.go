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
	dst Destination
}

func (s *source) Init(_ context.Context) error {
	// TODO(mwhittaker): Remove the nolint once golangci-lint supports go 1.21.
	s.Logger().Debug("simple.Init") //nolint:typecheck //nolint:nolintlint
	dst, err := weaver.Get[Destination](s)
	s.dst = dst
	return err
}

func (s *source) Emit(ctx context.Context, file, msg string) error {
	return s.dst.Record(ctx, file, msg)
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

func (d *destination) Getpid(_ context.Context) (int, error) {
	return os.Getpid(), nil
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
