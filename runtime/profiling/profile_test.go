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

package profiling_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/ServiceWeaver/weaver/runtime/profiling"
	"github.com/google/pprof/profile"
)

func TestProfileGroups(t *testing.T) {
	type testCase struct {
		name   string
		groups [][]func() ([]byte, error)
		expect int64
		errors []string // List of expected errors
	}
	for _, c := range []testCase{
		{
			"no_groups",
			nil,
			0,
			nil,
		},
		{
			"single_group",
			[][]func() ([]byte, error){
				{fakeProfile(100)},
			},
			100,
			nil,
		},
		{
			"multiple_groups",
			[][]func() ([]byte, error){
				{fakeProfile(100)},
				{fakeProfile(200)},
				{fakeProfile(300)},
			},
			600,
			nil,
		},
		{
			"with_empty_group",
			[][]func() ([]byte, error){
				{fakeProfile(100)},
				{},
			},
			100,
			nil,
		},
		{
			"multiple_replicas",
			[][]func() ([]byte, error){
				{fakeProfile(100), fakeProfile(100), fakeProfile(100)},
			},
			300,
			nil,
		},
		{
			"error",
			[][]func() ([]byte, error){
				{makeError("foo")},
			},
			0,
			[]string{"foo"},
		},
		{
			"partial_error",
			[][]func() ([]byte, error){
				{fakeProfile(100)},
				{makeError("foo")},
			},
			100,
			[]string{"foo"},
		},
		{
			"bad_replica_skipped",
			[][]func() ([]byte, error){
				{makeError("foo"), fakeProfile(100)},
			},
			200,
			[]string{"foo"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, err := profiling.ProfileGroups(c.groups)
			if n := sum(p); n != c.expect {
				t.Errorf("profile has sum %d, expecting %d", n, c.expect)
			}
			for _, want := range c.errors {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %v does not contain expected %q", err.Error(), want)
				}
			}
		})
	}
}

// fakeProfile returns a function that returns a synthetic profile with the specified total value.
func fakeProfile(value int64) func() ([]byte, error) {
	// Dummy profile constituents
	mapping := &profile.Mapping{
		ID:           1,
		Start:        0,
		Limit:        1000,
		Offset:       0,
		File:         "binary",
		HasFunctions: true,
	}
	fn := &profile.Function{
		ID:         1,
		Name:       "Foo",
		SystemName: "Foo",
	}
	loc := &profile.Location{
		ID:      1,
		Mapping: mapping,
		Address: 1,
		Line:    []profile.Line{{Function: fn}},
	}
	p := &profile.Profile{
		SampleType: []*profile.ValueType{
			{Type: "inuse_space", Unit: "bytes"},
		},
		DefaultSampleType: "inuse_space",
		Sample: []*profile.Sample{
			{Location: []*profile.Location{loc}, Value: []int64{value}},
		},
		Mapping:  []*profile.Mapping{mapping},
		Location: []*profile.Location{loc},
		Function: []*profile.Function{fn},
	}
	var buf bytes.Buffer
	if err := p.Write(&buf); err != nil {
		panic(err)
	}
	return func() ([]byte, error) {
		return buf.Bytes(), nil
	}
}

// sum returns the sum of the sample counts in a profile.
func sum(data []byte) int64 {
	if len(data) == 0 {
		return 0
	}
	prof, err := profile.ParseData(data)
	if err != nil {
		panic(err)
	}
	var result int64
	for _, s := range prof.Sample {
		result += s.Value[0]
	}
	return result
}

// makeError returns a function that generates an error when invoked.
func makeError(msg string) func() ([]byte, error) {
	return func() ([]byte, error) {
		return nil, fmt.Errorf("%s", msg)
	}
}
