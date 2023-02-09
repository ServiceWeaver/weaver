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

package foo

import (
	"context"

	"github.com/ServiceWeaver/weaver"
)

type byValue struct{ notSerializable chan int }
type byPointer struct{ notSerializable chan int }
type mixed1 struct{ notSerializable chan int }
type mixed2 struct{ notSerializable chan int }

func (byValue) MarshalBinary() ([]byte, error)    { return nil, nil }
func (byValue) UnmarshalBinary([]byte) error      { return nil }
func (*byPointer) MarshalBinary() ([]byte, error) { return nil, nil }
func (*byPointer) UnmarshalBinary([]byte) error   { return nil }
func (mixed1) MarshalBinary() ([]byte, error)     { return nil, nil }
func (*mixed1) UnmarshalBinary([]byte) error      { return nil }
func (*mixed2) MarshalBinary() ([]byte, error)    { return nil, nil }
func (mixed2) UnmarshalBinary([]byte) error       { return nil }

type foo interface {
	M(context.Context, byValue, byPointer, mixed1, mixed2) error
}

type impl struct{ weaver.Implements[foo] }

func (impl) M(context.Context, byValue, byPointer, mixed1, mixed2) error { return nil }
