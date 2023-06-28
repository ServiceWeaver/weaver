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

// See TestExampleVersion in generator_test.go.

package main

import (
	"context"

	"github.com/ServiceWeaver/weaver"
)

//go:generate ../../../../cmd/weaver/weaver generate

type message struct {
	weaver.AutoMarshal
	a int
	b string
	c bool
	d [10]int
	e []string
	f map[bool]int
}

type pair struct {
	weaver.AutoMarshal
	a message
	b message
}

type config struct {
	A int
	B string
	C bool
	D [10]int
	E []string
	F map[bool]int
}

type A interface {
	M1(context.Context, int, string, bool, [10]int, []string, map[bool]int, message) (pair, error)
	M2(context.Context, int, string, bool, [10]int, []string, map[bool]int, message) (pair, error)
}

type B interface {
	M1(context.Context, int, string, bool, [10]int, []string, map[bool]int, message) (pair, error)
	M2(context.Context, int, string, bool, [10]int, []string, map[bool]int, message) (pair, error)
}

type a struct {
	weaver.Implements[A]
	b    weaver.Ref[B]   //nolint:unused
	lis1 weaver.Listener `weaver:"renamed_listener"` //nolint:unused
	lis2 weaver.Listener //nolint:unused
	weaver.WithConfig[config]
	weaver.WithRouter[router]
}

type b struct {
	weaver.Implements[B]
	a    weaver.Ref[A]   //nolint:unused
	lis1 weaver.Listener `weaver:"renamed_listener"` //nolint:unused
	lis2 weaver.Listener //nolint:unused
	weaver.WithConfig[config]
	weaver.WithRouter[router]
}

func (a) M1(context.Context, int, string, bool, [10]int, []string, map[bool]int, message) (pair, error) {
	return pair{}, nil
}
func (a) M2(context.Context, int, string, bool, [10]int, []string, map[bool]int, message) (pair, error) {
	return pair{}, nil
}
func (b) M1(context.Context, int, string, bool, [10]int, []string, map[bool]int, message) (pair, error) {
	return pair{}, nil
}
func (b) M2(context.Context, int, string, bool, [10]int, []string, map[bool]int, message) (pair, error) {
	return pair{}, nil
}

type routingKey struct {
	weaver.AutoMarshal
	a int
	b string
	c float32
}

type router struct{}

func (router) M1(context.Context, int, string, bool, [10]int, []string, map[bool]int, message) routingKey {
	return routingKey{}
}
func (router) M2(context.Context, int, string, bool, [10]int, []string, map[bool]int, message) routingKey {
	return routingKey{}
}

func main() {}
