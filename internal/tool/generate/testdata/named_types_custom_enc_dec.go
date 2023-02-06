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

// EXPECTED
// EncodeBinaryMarshaler
// DecodeBinaryUnmarshaler

// UNEXPECTED
// Preallocate
// func serviceweaver_enc_X
// func serviceweaver_dec_X

// Methods with named types that have custom enc/dec functions.
package foo

import (
	"context"
	"time"

	"github.com/ServiceWeaver/weaver"
)

// struct with local implementation of the (Un)marshalBinary interface.
type X struct {
	notSerializable chan int
}

func (x X) MarshalBinary() ([]byte, error) {
	return nil, nil
}

func (x *X) UnmarshalBinary([]byte) error {
	return nil
}

type foo interface {
	A(context.Context, []X, time.Time) (X, error)
	B(context.Context, []X) error
}

type impl struct{ weaver.Implements[foo] }

func (l *impl) A(context.Context, []X, time.Time) (X, error) {
	return X{}, nil
}

func (l *impl) B(context.Context, []X) error {
	return nil
}
