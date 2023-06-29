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

import "github.com/ServiceWeaver/weaver/runtime/version"

// The following types are used to check, at compile time, that every
// weaver_gen.go file uses the codegen API version that is linked into the
// binary.
//
// It is best explained via an example. Imagine 'weaver generate' is running
// with codegen version 0.1.0. For every package, 'weaver generate' generates
// the following line:
//
//     var _ codegen.LatestVersion = codegen.Version[[0][1]struct{}]("...")
//
// This line implicitly checks that the type codegen.LatestVersion is equal to
// the type codegen.Version[[0][1]struct{}]. Imagine we build this code but
// link in codegen API version 0.2.0. When we compile, codegen.LatestVersion is
// an alias of Version[[0][2]struct{}] which is not equal to the
// Version[[0][1]struct{}] that appears in the generated code. This causes a
// compile error which includes a diagnostic message explaining what went wrong
// (this is "..." above). Again note that we ignore the patch number.

type Version[_ any] string
type LatestVersion = Version[[version.CodegenMajor][version.CodegenMinor]struct{}]
