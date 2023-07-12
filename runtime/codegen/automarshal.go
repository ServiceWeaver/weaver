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

package codegen

import (
	"fmt"
	"reflect"
	"sync"
)

// AutoMarshal is the interface implemented by structs with weaver.AutoMarshal
// declarations.
type AutoMarshal interface {
	WeaverMarshal(enc *Encoder)
	WeaverUnmarshal(dec *Decoder)
}

// autoMarshalPointer[T] is an interface which asserts that *T implements AutoMarshal.
//
//nolint:unused
type autoMarshalPointer[T any] interface {
	*T
	AutoMarshal
}

// Table of registered serialized types.
var (
	typesMu  sync.Mutex
	types    map[string]reflect.Type // Registered serializable types
	typeKeys map[reflect.Type]string // Keys of registered serializable types
)

// RegisterSerializable records type T as serializable. This is needed to
// instantiate the appropriate concrete type when an interface is sent over the
// wire (currently only used for AutoMarshal errors returned from remote method
// calls). The registration is automatically done by generated code for custom
// error structs that embed weaver.AutoMarshal.
//
// The extra PT type parameter is used to check that *T is an AutoMarshal
// (since AutoMarshal methods have pointer receivers, but T itself may be a
// struct).
// func RegisterSerializable[T any, PT autoMarshalPointer[T]]() {
func RegisterSerializable[T any]() {
	var value T
	t := reflect.TypeOf(value)
	typesMu.Lock()
	defer typesMu.Unlock()
	if types == nil {
		types = map[string]reflect.Type{}
		typeKeys = map[reflect.Type]string{}
	}
	key := typeKey(value)
	if existing, ok := types[key]; ok {
		if existing == t {
			return
		}
		panic(fmt.Sprintf("multiples types (%v and %v) have the same type string %q", existing, t, key))
	}
	types[key] = t
	typeKeys[t] = key
}

// typeKey returns the key to use to identify the type of value.
// The returned key is stable across processes.
func typeKey(value any) string {
	t := reflect.TypeOf(value)
	return t.PkgPath() + "." + t.Name()
}

// canEncodeInterface returns true if the concrete type of value is registered
// as a serializable type and can therefore be sent using Encoder.Interface.
func canEncodeInterface(value any) bool {
	key := typeKey(value)
	typesMu.Lock()
	defer typesMu.Unlock()
	_, ok := types[key]
	return ok
}
