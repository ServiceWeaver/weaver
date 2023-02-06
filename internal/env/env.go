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

// Package env implements helper functions for dealing with environment variables.
package env

import (
	"fmt"
	"strings"
)

// Split splits an environment variable of the form key=value into its
// constituent key and value parts.
func Split(kv string) (string, string, error) {
	k, v, ok := strings.Cut(kv, "=")
	if !ok {
		return "", "", fmt.Errorf("env: %q is not of form key=value", kv)
	}
	return k, v, nil
}

// Parse parses a list of environment variables of the form key=value into a
// map from keys to values. If a key appears multiple times, the last value of
// the key is returned.
func Parse(env []string) (map[string]string, error) {
	kvs := map[string]string{}
	for _, kv := range env {
		k, v, err := Split(kv)
		if err != nil {
			return nil, err
		}
		kvs[k] = v
	}
	return kvs, nil
}
