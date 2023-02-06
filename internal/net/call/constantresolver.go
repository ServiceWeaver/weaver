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

package call

import (
	"context"
	"fmt"
)

// constantResolver is a trivial constant resolver that returns a fixed set of
// endponts.
type constantResolver struct {
	endpoints []Endpoint
}

var _ Resolver = &constantResolver{}

// NewConstantResolver returns a new resolver that returns the provided
// set of endpoints.
func NewConstantResolver(endpoints ...Endpoint) Resolver {
	return &constantResolver{endpoints: endpoints}
}

// IsConstant implements the Resolver interface.
func (*constantResolver) IsConstant() bool {
	return true
}

// Resolve implements the Resolver interface.
func (c *constantResolver) Resolve(_ context.Context, version *Version) ([]Endpoint, *Version, error) {
	if version != nil {
		return nil, nil, fmt.Errorf("unexpected non-nil version %v", *version)
	}
	return c.endpoints, nil, nil
}
