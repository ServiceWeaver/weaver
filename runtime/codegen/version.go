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

package codegen

import "fmt"

const (
	Major = 0
	Minor = 9
	Patch = 0
)

func ReportVersion(major, minor, patch int) {
	if major != Major || minor != Minor || patch != Patch {
		err := fmt.Errorf(`You used 'weaver generate' codegen version %d.%d.%d, but you built your code with an incompatible weaver module version with codegen version %d.%d.%d. Try upgrading and re-running 'weaver generate'.`,
			major, minor, patch,
			Major, Minor, Patch,
		)
		panic(err)
	}
}
