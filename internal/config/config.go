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

// Package config contains config related utilities.
package config

import (
	"fmt"
	"reflect"
	"strings"
)

// Config calls the WithConfig.Config method on the provided value and returns
// the result. If the provided value doesn't have a WithConfig.Config method,
// Config returns nil.
//
// Config panics if the provided value is not a pointer to a struct.
func Config(v reflect.Value) any {
	// TODO(mwhittaker): Delete this function and use weaver.GetConfig instead.
	// Right now, there are some cyclic dependencies preventing us from doing
	// this.
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		panic(fmt.Errorf("invalid non pointer to struct value: %v", v))
	}
	s := v.Elem()
	t := s.Type()
	for i := 0; i < t.NumField(); i++ {
		// Check that f is an embedded field of type weaver.WithConfig[T].
		f := t.Field(i)
		if !f.Anonymous ||
			f.Type.PkgPath() != "github.com/ServiceWeaver/weaver" ||
			!strings.HasPrefix(f.Type.Name(), "WithConfig[") {
			continue
		}

		// Call the Config method to get a *T.
		config := s.Field(i).Addr().MethodByName("Config")
		return config.Call(nil)[0].Interface()
	}
	return nil
}
