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

// Package version contains the version of the weaver module and its
// constituent APIs (e.g., the pipe API, the codegen API).
package version

import (
	"fmt"
)

// TODO(mwhittaker): Write a doc explaining versioning in detail. Include
// Srdjan's comments in PR #219.

const (
	// The version of the deployer API.
	//
	// Every time we make a change to the deployer API, we assign it a new
	// version. We could assign the deployer API versions v1, v2, v3, and so
	// on. However, this makes it hard to understand the relationship between
	// the deployer API version and the version of the Service Weaver module.
	//
	// Instead, we use Service Weaver module versions as deployer API versions.
	// For example, if we change the deployer API in v0.12.0 of Service Weaver,
	// then we update the deployer API version to v0.12.0. If we don't change
	// the deployer API in v0.13.0 of Service Weaver, then we leave the
	// deployer API at v0.12.0.
	DeployerMajor = 0
	DeployerMinor = 22

	// The version of the codegen API. As with the deployer API, we assign a
	// new version every time we change how code is generated, and we use
	// weaver module versions.
	CodegenMajor = 0
	CodegenMinor = 20
)

var (
	// The deployer API version.
	DeployerVersion = SemVer{DeployerMajor, DeployerMinor, 0}

	// The codegen API version.
	CodegenVersion = SemVer{CodegenMajor, CodegenMinor, 0}
)

// SemVer is a semantic version. See https://go.dev/doc/modules/version-numbers
// for details.
type SemVer struct {
	Major int
	Minor int
	Patch int
}

func (s SemVer) String() string {
	return fmt.Sprintf("v%d.%d.%d", s.Major, s.Minor, s.Patch)
}
