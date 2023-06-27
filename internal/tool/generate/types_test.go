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

package generate

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
)

func TestCheckSerializable(t *testing.T) {
	type testCase struct {
		label    string
		contents string
		error    string // If empty, should succeed; else must be a sub-string of a a returned error
	}
	for _, c := range []testCase{
		// Serializable types:
		{"int", "type target int", ""},
		{"int8", "type target int8", ""},
		{"int16", "type target int16", ""},
		{"int32", "type target int32", ""},
		{"int64", "type target int64", ""},
		{"uint", "type target uint", ""},
		{"uint8", "type target uint8", ""},
		{"uint16", "type target uint16", ""},
		{"uint32", "type target uint32", ""},
		{"uint64", "type target uint64", ""},
		{"float32", "type target float32", ""},
		{"float64", "type target float64", ""},
		{"complex64", "type target complex64", ""},
		{"complex128", "type target complex128", ""},
		{"string", "type target string", ""},
		{"alias", "type other int; type target = other;", ""},
		{"array", "type target [16]byte", ""},
		{"slice", "type target []byte", ""},
		{"map", "type target map[string]bool", ""},
		{"pointer", "type target *int", ""},
		{"otherpkg", `
import "time"

type target time.Duration
`, ""},
		{"BinaryMarshaler", `
type target struct{}
func (t *target) MarshalBinary() ([]byte, error) { return nil, nil }
func (t *target) UnmarshalBinary([]byte) error { return nil }
`, ""},
		{"BinaryMarshaler by value", `
type target struct{}
func (t target) MarshalBinary() ([]byte, error) { return nil, nil }
func (t target) UnmarshalBinary([]byte) error { return nil }
`, ""},
		{"BinaryMarshaler mixed by value", `
type target struct{}
func (t target) MarshalBinary() ([]byte, error) { return nil, nil }
func (t *target) UnmarshalBinary([]byte) error { return nil }
`, ""},
		{"BinaryMarshaler recursive", `
type target struct { next *target }
func (t *target) MarshalBinary() ([]byte, error) { return nil, nil }
func (t *target) UnmarshalBinary([]byte) error { return nil }
`, ""},

		// Non-serializable types:
		{"function", "type target func()", "not a serializable type"},
		{"chan", "type target chan int", "not a serializable type"},
		{"struct", "type target struct{}", "not serializable"},
		{"nested struct", "type target []struct{}", "struct literals are not serializable"},
		{"missing unmarshal", `
type target struct{
	notSerializable chan int
}
func (t *target) MarshalBinary() ([]byte, error) { return nil, nil }
`, "not serializable"},
		{"missing marshal", `
type target struct{
	notSerializable chan int
}
func (t *target) UnmarshalBinary([]byte) error { return nil }
`, "not serializable"},
		{"interface", `
type target interface{
	MarshalBinary() ([]byte, error)
	UnmarshalBinary([]byte) error
}
`, "not currently supported"},
		{"simple recursive", `
type target *target
`, "not currently supported"},
		{"nested recursive", `
type target []*target
`, "not currently supported"},
		{"mutually recursive", `
type A []*B
type B []*A
type target A
`, "not currently supported"},
	} {
		t.Run(c.label, func(t *testing.T) {
			tset, target := compile(t, c.contents)
			errs := tset.checkSerializable(target)

			// Verify success or error as required.
			if c.error == "" {
				if len(errs) != 0 {
					t.Errorf("unexpected errors %v", errs)
				}
			} else {
				found := false
				for _, err := range errs {
					if strings.Contains(err.Error(), c.error) {
						found = true
					}
				}
				if !found {
					t.Errorf("error %q not found in %v", c.error, errs)
				}
			}
		})
	}
}

func TestSizeOf(t *testing.T) {
	type testCase struct {
		label    string
		contents string
		want     int
	}
	for _, c := range []testCase{
		// Fixed size.
		{"bool", "type target bool", 1},
		{"int", "type target int", 8},
		{"int8", "type target int8", 1},
		{"int16", "type target int16", 2},
		{"int32", "type target int32", 4},
		{"int64", "type target int64", 8},
		{"uint", "type target uint", 8},
		{"uint8", "type target uint8", 1},
		{"uint16", "type target uint16", 2},
		{"uint32", "type target uint32", 4},
		{"uint64", "type target uint64", 8},
		{"complex64", "type target complex64", 8},
		{"complex128", "type target complex128", 16},
		{"array", "type target [42]byte", 42},
		{"EmptyStruct", "type target struct{}", 0},
		{"SimpleStruct", "type target struct{x int; y bool}", 9},
		{"NestedStruct", `
type A struct { x B; y C }
type B struct { x int; y C }
type C struct { x int; y bool }
type target A`, 26},
		{"Generic", `
type Wrapper[A any] struct {Val A}
type target Wrapper[int]
`, 8},

		// Not fixed size.
		{"string", "type target string", -1},
		{"slice", "type target []byte", -1},
		{"pointer", "type target *int", -1},
		{"map", "type target map[int]int", -1},
		{"chan", "type target chan int", -1},
		{"func", "type target func() int", -1},
		{"NonFixedStruct", "type target struct{x int; y string}", -1},
		{"NestedNonFixedStruct", `
type A struct { x B; y C }
type B struct { x int; y C }
type C struct { x int; y string }
type target A`, -1},
	} {
		t.Run(c.label, func(t *testing.T) {
			tset, target := compile(t, c.contents)
			if got, want := tset.sizeOfType(target), c.want; got != want {
				t.Fatalf("sizeOfType: got %v, want %v", got, want)
			}
		})
	}
}

func TestIsMeasurable(t *testing.T) {
	type testCase struct {
		label    string
		contents string
		want     bool
	}
	for _, c := range []testCase{
		// Fixed size.
		{"bool", "type target bool", true},
		{"int", "type target int", true},
		{"int8", "type target int8", true},
		{"int16", "type target int16", true},
		{"int32", "type target int32", true},
		{"int64", "type target int64", true},
		{"uint", "type target uint", true},
		{"uint8", "type target uint8", true},
		{"uint16", "type target uint16", true},
		{"uint32", "type target uint32", true},
		{"uint64", "type target uint64", true},
		{"complex64", "type target complex64", true},
		{"complex128", "type target complex128", true},
		{"[42]byte", "type target [42]byte", true},
		{"EmptyStruct", "type target struct{}", true},
		{"SimpleStruct", "type target struct{x int; y bool}", true},
		{"NestedStruct", `
type A struct { x B; y C }
type B struct { x int; y C }
type C struct { x int; y bool }
type target A`, true},
		{"Generic", `
type Wrapper[A any] struct {Val A}
type target Wrapper[int]
`, true},

		// Measurable, but not fixed size.
		{"string", "type target string", true},
		{"[]byte", "type target []byte", true},
		{"*int", "type target *int", true},
		{"*string", "type target *string", true},
		{"map[int]int", "type target map[int]int", true},

		// Not measurable.
		{"[1]string", "type target [1]string", false},
		{"[]string", "type target []string", false},
		{"map[int]string", "type target map[int]string", false},
		{"map[string]int", "type target map[string]int", false},
		{"map[string]string", "type target map[string]string", false},
		{"chan", "type target chan int", false},
		{"func", "type target func() int", false},
		{"OtherPackage", "type target time.Duration", false},
		{"NonMeasurableStruct", "type target struct{x int; y []string}", false},
		{"NestedNonMeasurableStruct", `
type A struct { x B; y C }
type B struct { x int; y C }
type C struct { x int; y []string }
type target A`, false},
	} {
		t.Run(c.label, func(t *testing.T) {
			tset, target := compile(t, c.contents)
			if got := tset.isMeasurable(target); got != c.want {
				t.Fatalf("isMeasurable: got %v, want %v", got, c.want)
			}
		})
	}
}

func TestIsValidRouterType(t *testing.T) {
	type testCase struct {
		label    string
		contents string
		want     bool
	}
	for _, c := range []testCase{
		{"int", "type target int", true},
		{"bool", "type target bool", false},
		{"array", "type target [42]byte", false},
		{"empty struct", "type target struct{}", true},
		{"simple struct", "type target struct{x int; y string}", true},
		{"embedded weaver.AutoMarshal", `
import "github.com/ServiceWeaver/weaver"
type target struct{
	weaver.AutoMarshal
	foo int
	bar string
}`, true},
		{"embedded with something else", `
import "github.com/ServiceWeaver/weaver"
type foo struct {}
type target struct {
	foo
	x int
	y int
}`, false},
	} {
		t.Run(c.label, func(t *testing.T) {
			_, target := compile(t, c.contents)
			if got := isValidRouterType(target); got != c.want {
				t.Fatalf("isValidRouterType: got %v, want %v", got, c.want)
			}
		})
	}
}

func compile(t *testing.T, contents string) (*typeSet, types.Type) {
	t.Helper()

	// Set up directory for packages.Load.
	tmp := t.TempDir()
	save := func(f, data string) {
		if err := os.WriteFile(filepath.Join(tmp, f), []byte(data), 0644); err != nil {
			t.Fatalf("error writing %s: %v", f, err)
		}
	}
	save("go.mod", goModFile)
	save("contents.go", "package foo\n"+contents)
	fset := token.NewFileSet()
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedSyntax |
			packages.NeedImports |
			packages.NeedTypes |
			packages.NeedTypesInfo,
		Dir:  tmp,
		Fset: fset,
	}

	// Load package.
	pkgs, err := packages.Load(cfg, "file="+filepath.Join(tmp, "contents.go"))
	if err != nil {
		t.Fatal(err)
	}

	// Find the type we are going to check.
	target, err := findType(pkgs[0], "target")
	if err != nil {
		t.Fatal(err)
	}
	var automarshals typeutil.Map
	var automarshalCandidates typeutil.Map
	tset := newTypeSet(pkgs[0], &automarshals, &automarshalCandidates)
	return tset, target
}

func findType(pkg *packages.Package, name string) (types.Type, error) {
	for _, f := range pkg.Syntax {
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok {
				continue
			}
			if gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				v, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if v.Name.Name != name {
					continue
				}
				return pkg.TypesInfo.TypeOf(v.Name), nil
			}
		}
	}
	return nil, fmt.Errorf("%s not found", name)
}
