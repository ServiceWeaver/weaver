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

package tool_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/google/pprof/profile"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/ServiceWeaver/weaver/runtime/tool"
)

func TestProfileGroups(t *testing.T) {
	type testCase struct {
		name   string
		groups [][]func() (*protos.Profile, error)
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
			[][]func() (*protos.Profile, error){
				{fakeProfile(100)},
			},
			100,
			nil,
		},
		{
			"multiple_groups",
			[][]func() (*protos.Profile, error){
				{fakeProfile(100)},
				{fakeProfile(200)},
				{fakeProfile(300)},
			},
			600,
			nil,
		},
		{
			"with_empty_group",
			[][]func() (*protos.Profile, error){
				{fakeProfile(100)},
				{},
			},
			100,
			nil,
		},
		{
			"multiple_replicas",
			[][]func() (*protos.Profile, error){
				{fakeProfile(100), fakeProfile(100), fakeProfile(100)},
			},
			300,
			nil,
		},
		{
			"error",
			[][]func() (*protos.Profile, error){
				{makeError("foo")},
			},
			0,
			[]string{"foo"},
		},
		{
			"partial_error",
			[][]func() (*protos.Profile, error){
				{fakeProfile(100)},
				{makeError("foo")},
			},
			100,
			[]string{"foo"},
		},
		{
			"bad_replica_skipped",
			[][]func() (*protos.Profile, error){
				{makeError("foo"), fakeProfile(100)},
			},
			200,
			[]string{"foo"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, err := tool.ProfileGroups(c.groups)
			var gotErrors []string
			if err != nil {
				gotErrors = append(gotErrors, err.Error())
			}
			if p != nil {
				gotErrors = append(gotErrors, p.Errors...)
			}
			if n := sum(p); n != c.expect {
				t.Errorf("profile has sum %d, expecting %d", n, c.expect)
			}
			for i := 0; i < len(c.errors) && i < len(gotErrors); i++ {
				if !strings.Contains(gotErrors[i], c.errors[i]) {
					t.Errorf("error %v does not contain expected %q", gotErrors[i], c.errors[i])
				}
			}
			for i := len(c.errors); i < len(gotErrors); i++ {
				t.Errorf("unexpected error %v", gotErrors[i])
			}
			for i := len(gotErrors); i < len(c.errors); i++ {
				t.Errorf("missing error %v", c.errors[i])
			}
		})
	}
}

// fakeProfile returns a function that returns a synthetic profile with the specified total value.
func fakeProfile(value int64) func() (*protos.Profile, error) {
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
	return func() (*protos.Profile, error) {
		return &protos.Profile{Data: buf.Bytes()}, nil
	}
}

// sum returns the sum of the sample counts in a profile.
func sum(p *protos.Profile) int64 {
	if p == nil || len(p.Data) == 0 {
		return 0
	}
	prof, err := profile.ParseData(p.Data)
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
func makeError(msg string) func() (*protos.Profile, error) {
	return func() (*protos.Profile, error) {
		return nil, fmt.Errorf("%s", msg)
	}
}
