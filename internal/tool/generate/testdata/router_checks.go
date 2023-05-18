// Copyright 2023 Google LLC
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

// EXPECTED
// var _ weaver.Unrouted = (*unrouted)(nil)
// var _ weaver.RoutedBy[router] = (*routed)(nil)
// var _ func(context.Context) int = (&router{}).A
// var _ = (&__routed_router_if_youre_seeing_this_you_probably_forgot_to_run_weaver_generate{}).B
package foo

import (
	"context"

	"github.com/ServiceWeaver/weaver"
)

type Unrouted interface{}

type unrouted struct {
	weaver.Implements[Unrouted]
}

type Routed interface {
	A(context.Context) error
	B(context.Context) error
}

type routed struct {
	weaver.Implements[Routed]
	weaver.WithRouter[router]
}

func (routed) A(context.Context) error { return nil }
func (routed) B(context.Context) error { return nil }

type router struct{}

func (router) A(context.Context) int { return 42 }
