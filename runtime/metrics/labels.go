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

package metrics

import (
	"fmt"
	"reflect"
	"unicode"
	"unicode/utf8"
)

// unexport returns a copy of s with the first letter lowercased.
func unexport(s string) string {
	// NOTE(mwhittaker): Handling unicode complicates the implementation of
	// this function. I took this implementation from [1].
	//
	// [1]: https://groups.google.com/g/golang-nuts/c/WfpmVDQFecU/m/-1IBD5KI7GEJ.
	if s == "" {
		return ""
	}
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToLower(r)) + s[n:]
}

// typecheckLabels checks that L is a valid label struct type. See metricMap
// for a description of valid label struct types.
func typecheckLabels[L comparable]() error {
	var x L
	t := reflect.TypeOf(x)
	if t.Kind() != reflect.Struct {
		return fmt.Errorf("metric labels: type %T is not a struct", x)
	}

	names := map[string]struct{}{}
	for i := 0; i < t.NumField(); i++ {
		fi := t.Field(i)

		// Check the type.
		if fi.Type.PkgPath() != "" {
			// Avoid named types like `type foo string`
			return fmt.Errorf("metric labels: field %q of type %T has unsupported type %v", fi.Name, x, fi.Type.Name())
		}
		switch fi.Type.Kind() {
		case reflect.String,
			reflect.Bool,
			reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		default:
			return fmt.Errorf("metric labels: field %q of type %T has unsupported type %v", fi.Name, x, fi.Type.Name())
		}

		// Check the visibility.
		if !fi.IsExported() {
			return fmt.Errorf("metric labels: field %q of type %T is unexported", fi.Name, x)
		}

		// Check for duplicate fields.
		name := unexport(fi.Name)
		if alias, ok := fi.Tag.Lookup("weaver"); ok {
			name = alias
		}
		if _, ok := names[name]; ok {
			return fmt.Errorf("metric labels: type %T has duplicate field %q", x, fi.Name)
		}
		names[name] = struct{}{}
	}

	return nil
}

// labelExtractor extracts labels from a label struct of type L.
type labelExtractor[L comparable] struct {
	fields []field
}

type field struct {
	f    reflect.StructField // struct field
	name string              // field name, or alias if present
}

// newLabelExtractor returns a new labelExtractor that can extract the labels
// from a label struct of type L. L must be a valid label struct type.
func newLabelExtractor[L comparable]() *labelExtractor[L] {
	var x L
	t := reflect.TypeOf(x)
	fields := make([]field, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		fi := t.Field(i)
		if alias, ok := fi.Tag.Lookup("weaver"); ok {
			fields[i] = field{fi, alias}
		} else {
			fields[i] = field{fi, unexport(fi.Name)}
		}
	}
	return &labelExtractor[L]{fields}
}

// Extract extracts the labels from a label struct. The provided labels must be
// the same type used to construct the labelExtractor.
func (l *labelExtractor[L]) Extract(labels L) map[string]string {
	v := reflect.ValueOf(labels)
	extracted := map[string]string{}
	for _, field := range l.fields {
		extracted[field.name] = fmt.Sprint(v.FieldByIndex(field.f.Index).Interface())
	}
	return extracted
}
