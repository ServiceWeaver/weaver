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

package runtime

const (
	// The version of the deployer API---aka pipe API---in semantic version
	// format (Major.Minor.Patch).
	//
	// Every time we make a change to deployer API, we assign it a new version.
	// When an envelope spawns a weavelet, the weavelet reports this version to
	// the envelope. The envelope then errors out if it is not compatible with
	// the reported version.
	//
	// We could assign the deployer API versions v1, v2, v3, and so on.
	// However, this makes it hard to understand the relationship between the
	// deployer API version and the version of the Service Weaver module. (What
	// version of Service Weaver do I need to install to get version 7 of the
	// pipe?)
	//
	// Instead, we use Service Weaver module versions as deployer API versions.
	// For example, if we change the deployer API in v0.12.0 of Service Weaver,
	// then we update the deployer API version to v0.12.0. If we don't change
	// the deployer API in v0.13.0 of Service Weaver, then we leave the
	// deployer API at v0.12.0.
	//
	// TODO(mwhittaker): Write a doc explaining versioning in detail. Include
	// Srdjan's comments in PR #219.
	Major = 0
	Minor = 6
	Patch = 0
)
