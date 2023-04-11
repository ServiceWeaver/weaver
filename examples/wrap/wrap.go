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

package main

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/metrics"
)

var (
	// stringLength is a histogram that tracks the length of strings passed to
	// the Wrap method.
	stringLength = metrics.NewHistogram(
		"wrap_string_length",
		"The length of strings passed to Wrap",
		metrics.NonNegativeBuckets,
	)

	// lineLength is a histogram that tracks the line length, n, passed to the
	// Wrap method.
	lineLength = metrics.NewHistogram(
		"wrap_line_length",
		"The line length, n, passed to Wrap",
		metrics.NonNegativeBuckets,
	)
)

// The Wrapper component interface.
type Wrapper interface {
	// Wrap wraps the provided string to lines of the provided length. For
	// example, we can wrap the following text:
	//
	//     Do you like green eggs and ham? I do not like them, Sam-I-am. I do
	//     not like green eggs and ham.
	//
	// to lines of length 20:
	//
	//     Do you like green
	//     eggs and ham? I do
	//     not like them,
	//     Sam-I-am. I do not
	//     like green eggs and
	//     ham.
	Wrap(context.Context, string, int) (string, error)
}

// The Wrapper component implementation.
type wrapper struct {
	weaver.Implements[Wrapper]
}

// Wrap wraps the provided text to n characters.
func (wrapper) Wrap(_ context.Context, s string, n int) (string, error) {
	stringLength.Put(float64(len(s))) // Update the stringLength metric.
	lineLength.Put(float64(n))        // Update the lineLength metric.

	var b strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		fmt.Fprintln(&b, wrapline(scanner.Text(), n))
	}
	return b.String(), nil
}

// wrapline wraps the provided line to n characters.
func wrapline(line string, n int) string {
	// firstRune returns the first rune of s, or panics if s is empty.
	firstRune := func(s string) rune {
		for _, r := range s {
			return r
		}
		panic(fmt.Errorf("firstRune: empty string"))
	}

	var b strings.Builder
	length := 0
	parts := split(line)
	for i, part := range parts {
		if length == 0 || length+len(part) <= n {
			b.WriteString(part)
			length += len(part)
		} else if unicode.IsSpace(firstRune(part)) {
			if i != len(parts)-1 {
				b.WriteString("\n")
				length = 0
			}
		} else {
			b.WriteString("\n")
			b.WriteString(part)
			length = len(part)
		}
	}
	return b.String()
}

// split splits s into abutting substrings of wholly whitespace or wholly
// non-whitespace characters. For example, split splits " foo \t bar " into
// " ", "foo", " \t ",  "bar", and " ".
func split(s string) []string {
	var parts []string
	var part []rune
	for _, r := range s {
		if len(part) == 0 || unicode.IsSpace(r) == unicode.IsSpace(part[0]) {
			part = append(part, r)
		} else {
			parts = append(parts, string(part))
			part = []rune{r}
		}
	}
	if len(part) > 0 {
		parts = append(parts, string(part))
	}
	return parts
}
