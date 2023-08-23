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

// ERROR: multiple listeners with name bar

// Multiple listeners with the same name.
package foo

import (
	"github.com/ServiceWeaver/weaver"
)

type foo interface{}

type impl struct {
	weaver.Implements[foo]
	bar weaver.Listener
	_   weaver.Listener `weaver:"bar"`
}
