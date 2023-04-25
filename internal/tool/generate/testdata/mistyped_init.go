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

package foo

import (
	"context"

	"github.com/ServiceWeaver/weaver"
)

type NoContext interface{}
type NoError interface{}
type NoContextOrError interface{}
type ExtraArgs interface{}
type ExtraReturns interface{}

type noContext struct {
	weaver.Implements[NoContext]
}
type noError struct {
	weaver.Implements[NoError]
}
type noContextOrError struct {
	weaver.Implements[NoContextOrError]
}
type extraArgs struct {
	weaver.Implements[ExtraArgs]
}
type extraReturns struct {
	weaver.Implements[ExtraReturns]
}

func (noContext) Init() error                          { return nil }
func (noError) Init(context.Context)                   {}
func (noContextOrError) Init()                         {}
func (extraArgs) Init(context.Context, int) error      { return nil }
func (extraReturns) Init(context.Context) (int, error) { return 42, nil }
