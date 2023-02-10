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

package colors

import (
	"os"
	"testing"
)

func TestTabularizer(t *testing.T) {
	red := Color256(124)
	blue := Color256(27)
	green := Color256(40)

	title := []Text{
		{{S: "CATS", Bold: true}},
		{{S: "felis catus", Color: Color256(254)}},
	}
	tab := NewTabularizer(os.Stdout, title, NoDim)
	defer tab.Flush()
	tab.Row("NAME", "AGE", "COLOR")
	tab.Row("belle", "1y", Atom{S: "tortie", Color: red})
	tab.Row("sidney", "2y", Atom{S: "calico", Color: blue})
	tab.Row("dakota", "8m", Atom{S: "tuxedo", Color: green, Underline: true})
}
