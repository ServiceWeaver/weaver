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

package logtype

import "fmt"

// Logger is a subset of weaver.Logger so that lower-level modules do not need
// to depend on weaver.
// A Logger can safely be used concurrently from multiple goroutines.
type Logger interface {
	Debug(msg string, attrs ...any)
	Info(msg string, attrs ...any)
	Error(msg string, err error, attrs ...any)
	// We do not include the With() method in this interface since
	// its return type has to be weaver.Logger, which would cause
	// a dependency cycle.
}

// AppendAttrs appends <name,value> pairs found in attrs to prefix
// and returns the resulting slice. It never appends in place.
func AppendAttrs(prefix []string, attrs []any) []string {
	if len(attrs) == 0 {
		return prefix
	}

	// NOTE: Copy prefix to avoid the scenario where two different
	// loggers overwrite the existing slice entry. This is possible,
	// for example, if two goroutines call With() on the same logger
	// concurrently.
	dst := make([]string, 0, len(prefix)+len(attrs))
	dst = append(dst, prefix...)

	// Extract key,value pairs from attrs.
	for i, n := 0, len(attrs); i < n; {
		// We expect attrs[i] to be a string key and attrs[i+1] to be a value.
		// The two likely reasons for not meeting this expectation are:
		//    (1) User forgot the attr key.
		//    (2) User passed in the wrong type for the attr key.
		// (1) seems more likely, so assume a missing key.
		var val any
		key, ok := attrs[i].(string)
		if !ok || i+1 == n {
			key = "!BADKEY"
			val = attrs[i]
			i++
		} else {
			val = attrs[i+1]
			i += 2
		}
		dst = append(dst, key, fmt.Sprint(val))
	}
	return dst
}
