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
	"bufio"
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var (
	genFilesStorageDir = flag.String("generated_files_storage_dir", "",
		"If non-empty, generated files are persisted in the given directory.")

	testDir      string // the directory of this file
	weaverSrcDir string // the root directory of the Service Weaver repository
	goModFile    string // the go.mod file used in the tests
)

func init() {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("No caller information")
	}
	testDir = filepath.Dir(filename)
	weaverSrcDir = filepath.Join(testDir, "../../../")
	goModFile = fmt.Sprintf(`module "foo"

go 1.18

require github.com/ServiceWeaver/weaver v0.0.0
replace github.com/ServiceWeaver/weaver => %s
`, weaverSrcDir)
}

// runGenerator runs "weaver generate" on the provided file contents---originally
// in a file with the provided filename and directory---and returns the
// directory in which the code was compiled, the output of "weaver generate",
// and any errors. All provided subdirectories are also included in the call to
// "weaver generate".
//
// If "weaver generate" succeeds, the produced weaver_gen.go file is written in
// the provided directory with name ${filename}_weaver_gen.go.
func runGenerator(t *testing.T, directory, filename, contents string, subdirs []string) (string, error) {
	// runGenerator creates a temporary directory, copies the file and all
	// subdirs into it, writes a go.mod file, runs "go mod tidy", and finally
	// runs "weaver generate".
	//
	// TODO(mwhittaker): Double check paths are handled properly. What if we
	// run this test in another directory?
	tmp := t.TempDir()
	save := func(f, data string) {
		if err := os.WriteFile(filepath.Join(tmp, f), []byte(data), 0644); err != nil {
			t.Fatalf("error writing %s: %v", f, err)
		}
	}

	// Write the source code and go.mod files.
	save(filename, contents)
	save("go.mod", goModFile)

	// Copy over the subdirectories.
	for _, sub := range subdirs {
		cp := exec.Command("cp", "-r", sub, tmp)
		cp.Dir = directory
		cp.Stdout = os.Stdout
		cp.Stderr = os.Stderr
		if err := cp.Run(); err != nil {
			t.Fatalf("cp %q %q: %v", sub, tmp, err)
		}
	}

	// Run "go mod tidy"
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = tmp
	tidy.Stdout = os.Stdout
	tidy.Stderr = os.Stderr
	if err := tidy.Run(); err != nil {
		t.Fatalf("go mod tidy: %v", err)
	}

	// Run "weaver generate".
	opt := Options{
		Warn: func(err error) { t.Log(err) },
	}
	if err := Generate(tmp, []string{tmp}, opt); err != nil {
		return "", err
	}
	output, err := os.ReadFile(filepath.Join(tmp, generatedCodeFile))
	if err != nil {
		return "", err
	}

	if *genFilesStorageDir != "" {
		// Write the generated weaver_gen.go file to disk. The output for file
		// x.go is stored as x_weaver_gen.go.
		generated := strings.TrimSuffix(filename, ".go") + "_" + generatedCodeFile
		os.Remove(filepath.Join(directory, generated))
		if err := os.WriteFile(filepath.Join(*genFilesStorageDir, generated), output, 0600); err != nil {
			t.Fatalf("error writing the generated code for test %s: %v", filename, err)
		}
	}

	// Run "go mod tidy" and "go build" to make sure that weaver_gen.go
	// compiles.
	//
	// TODO(mwhittaker): Instead of running external commands, can we invoke
	// these using a builtin library?
	tidy = exec.Command("go", "mod", "tidy")
	tidy.Dir = tmp
	tidy.Stdout = os.Stdout
	tidy.Stderr = os.Stderr
	if err := tidy.Run(); err != nil {
		t.Fatalf("go mod tidy: %v", err)
	}
	gobuild := exec.Command("go", "build")
	gobuild.Dir = tmp
	gobuild.Stdout = os.Stdout
	gobuild.Stderr = os.Stderr
	if err := gobuild.Run(); err != nil {
		if testing.Verbose() {
			printOutput(t, output)
		}
		t.Fatalf("go build: %v", err)
	}

	return string(output), nil
}

func printOutput(t *testing.T, output []byte) {
	t.Log("Output:")
	for i, line := range bytes.Split(output, []byte("\n")) {
		t.Logf("%4d %s", i+1, string(line))
	}
}

// TestGenerator runs "weaver generate" on all of the files in testdata/. Every
// file in testdata/ must begin with a header that looks like this:
//
//	// EXPECTED
//	// a string
//	// another string
//
//	// UNEXPECTED
//	// don't expect this
//	// what a surprise
//
// This test runs "weaver generate" on the file and checks that every expected
// string appears in the generated weaver_gen.go file and that every unexpected
// string doesn't.
func TestGenerator(t *testing.T) {
	const dir = "testdata"
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("cannot list files in %q", dir)
	}

	for _, file := range files {
		filename := file.Name()
		if !strings.HasSuffix(filename, ".go") || strings.HasSuffix(filename, generatedCodeFile) {
			continue
		}
		t.Run(filename, func(t *testing.T) {
			t.Parallel()

			// Read the test file.
			bits, err := os.ReadFile(filepath.Join(dir, filename))
			if err != nil {
				t.Fatalf("cannot read %q: %v", filename, err)
			}
			contents := string(bits)

			// Parse the "EXPECTED" and "UNEXPECTED" blocks.
			var expected []string
			var unexpected []string
			parsingExpected := false
			parsingUnexpected := false
			scanner := bufio.NewScanner(bytes.NewBuffer(bits))
			for scanner.Scan() {
				line := scanner.Text()
				switch {
				case !strings.HasPrefix(line, "//"):
					parsingExpected = false
					parsingUnexpected = false
				case parsingExpected:
					expected = append(expected, strings.TrimPrefix(line, "// "))
				case parsingUnexpected:
					unexpected = append(unexpected, strings.TrimPrefix(line, "// "))
				case line == "// EXPECTED":
					parsingExpected = true
				case line == "// UNEXPECTED":
					parsingUnexpected = true
				}
			}
			if err := scanner.Err(); err != nil {
				t.Fatalf("error reading %q: %v", filename, err)
			}

			// Run "weaver generate".
			output, err := runGenerator(t, dir, filename, contents, []string{"sub1", "sub2"})
			if err != nil {
				t.Fatalf("error running generator: %v", err)
			}
			for _, expect := range expected {
				if strings.Contains(output, expect) {
					continue
				}
				t.Errorf("output does not contain expected string %q", expect)
			}
			for _, unexpect := range unexpected {
				if strings.Contains(output, unexpect) {
					t.Errorf("output contains unexpected string %q", unexpect)
				}
			}
		})
	}
}

// TestGeneratorErrors runs "weaver generate" on all of the files in
// testdata/errors.
// Every file in testdata/errors must begin with a single line header that looks
// like this:
//
//	// ERROR: bad file
//
// This test runs "weaver generate" on the file and checks that "weaver generate"
// produces an error that contains the provided string.
func TestGeneratorErrors(t *testing.T) {
	const dir = "testdata/errors"
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("cannot list files in %q", dir)
	}

	for _, file := range files {
		filename := file.Name()
		if !strings.HasSuffix(filename, ".go") || strings.HasSuffix(filename, generatedCodeFile) {
			continue
		}
		t.Run(filename, func(t *testing.T) {
			t.Parallel()

			// Read the test file.
			bits, err := os.ReadFile(filepath.Join(dir, filename))
			if err != nil {
				t.Fatalf("cannot read %q: %v", filename, err)
			}
			contents := string(bits)

			// Parse the "ERROR: ..." annotation.
			var want string
			scanner := bufio.NewScanner(bytes.NewBuffer(bits))
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "// ERROR: ") {
					want = strings.TrimPrefix(line, "// ERROR: ")
					break
				}
			}
			if err := scanner.Err(); err != nil {
				t.Fatalf("error reading %q: %v", filename, err)
			}
			if want == "" {
				t.Fatalf(`"ERROR: " annotation not found`)
			}

			// Run "weaver generate".
			output, err := runGenerator(t, dir, filename, contents, []string{})
			errfile := strings.TrimSuffix(filename, ".go") + "_error.txt"
			if err == nil {
				os.Remove(filepath.Join(dir, errfile))
				t.Fatalf("unexpectedly succeeded (want error containing %q):\n%s\n", want, output)
			}

			// Check that the error looks like what we expect.
			if !strings.Contains(err.Error(), want) {
				t.Errorf("bad errors: want %q, got: %s", want, err.Error())
			}
		})
	}
}

func TestSanitize(t *testing.T) {
	// Test plan: Check that sanitize returns the expected sanitized name for
	// various types. Also check that sanitize is injective; i.e. every type
	// gets mapped to a unique name.
	tests := []struct {
		name       string
		program    string
		underlying bool
		want       string
	}{
		{"Pointer", "type target *int", true, "ptr_int"},
		{"Slice", "type target []int", true, "slice_int"},
		{"Array", "type target [42]int", true, "array_42_int"},
		{"Map", "type target map[int]bool", true, "map_int_bool"},
		{"Int", "type target int", true, "int"},
		{"Int8", "type target int8", true, "int8"},
		{"Uint", "type target uint", true, "uint"},
		{"Uint8", "type target uint8", true, "uint8"},
		{"Float32", "type target float32", true, "float32"},
		{"Complex64", "type target complex64", true, "complex64"},
		{"String", "type target string", true, "string"},
		{"Named", "type target int", false, "target"},
		{"EmptyStruct", "type target struct{}", true, "struct"},
		{"SingletonStruct", "type target struct{x int}", true, "struct"},
		{"MultiStruct", "type target struct{x int; y bool}", true, "struct"},
		{"TaggedStruct", "type target struct{x int `foo bar`; y bool}", true, "struct"},
		{"EmbeddedStruct", "type target struct{int; y bool}", true, "struct"},
		{"TaggedEmbeddedStruct", "type target struct{int `foo`; y bool}", true, "struct"},
		{"Generic", `
		type One[X any] []X
		type Two[X, Y any] struct{x X; y Y}
		type target []Two[One[int], Two[bool, One[One[int]]]]
		`, true, "slice_Two_One_int_Two_bool_One_One_int"},
		{"Nested", `
		type A map[string][]map[bool][]int
		type B = map[string][]map[bool][]int
		type target map[A][]*B
		`, true, "map_A_slice_ptr_map_string_slice_map_bool_slice_int"},
		{"ComplexStruct", `
		type A int
		type target struct{
			int
			x A
			y *int
			z struct{x int; y bool}
		}
		`, true, "struct"},
	}

	sanitized := []string{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, typ := compile(t, test.program)
			if test.underlying {
				typ = typ.Underlying()
			}
			got := sanitize(typ)
			sanitized = append(sanitized, got)
			// The sanitized name ends in "_XXXXXXXX". We ignore this suffix.
			if len(got) <= 9 {
				t.Fatalf("sanitize(%v) too short", typ)
			}
			if got[:len(got)-9] != test.want {
				t.Fatalf("sanitize(%v): got %q, want %q", typ, got, test.want)
			}
		})
	}

	// Check that every name is unique.
	for i := 0; i < len(sanitized); i++ {
		for j := i + 1; j < len(sanitized); j++ {
			if sanitized[i] == sanitized[j] {
				t.Errorf("duplicate name %q", sanitized[i])
			}
		}
	}
}

func TestUniqueName(t *testing.T) {
	for _, test := range []struct {
		name       string
		program    string
		underlying bool
		want       string
	}{
		{"Pointer", "type target *int", true, "*int"},
		{"Slice", "type target []int", true, "[]int"},
		{"Array", "type target [42]int", true, "[42]int"},
		{"Map", "type target map[int]bool", true, "map[int]bool"},
		{"Int", "type target int", true, "int"},
		{"Int8", "type target int8", true, "int8"},
		{"Uint", "type target uint", true, "uint"},
		{"Uint8", "type target uint8", true, "uint8"},
		{"Float32", "type target float32", true, "float32"},
		{"Complex64", "type target complex64", true, "complex64"},
		{"String", "type target string", true, "string"},
		{"Named", "type target int", false, "Named(foo.target)"},
		{"EmptyStruct", "type target struct{}", true, "struct{}"},
		{"SingletonStruct", "type target struct{x int}", true, "struct{x int}"},
		{"MultiStruct", "type target struct{x int; y bool}", true, "struct{x int; y bool}"},
		{"TaggedStruct", "type target struct{x int `foo bar`; y bool}", true, "struct{x int `foo bar`; y bool}"},
		{"EmbeddedStruct", "type target struct{int; y bool}", true, "struct{int; y bool}"},
		{"TaggedEmbeddedStruct", "type target struct{int `foo`; y bool}", true, "struct{int `foo`; y bool}"},
		{"Generic", `
		type One[X any] []X
		type Two[X, Y any] struct{x X; y Y}
		type target []Two[One[int], Two[bool, One[One[int]]]]
		`, true, "[]Named(foo.Two)[Named(foo.One)[int], Named(foo.Two)[bool, Named(foo.One)[Named(foo.One)[int]]]]"},
		{"Nested", `
type A map[string][]map[bool][]int
type B = map[string][]map[bool][]int
type target map[A][]*B
`, true, "map[Named(foo.A)][]*map[string][]map[bool][]int"},
		{"ComplexStruct", `
type A int
type target struct{
	int
	x A
	y *int
	z struct{x int; y bool}
}
`, true, "struct{int; x Named(foo.A); y *int; z struct{x int; y bool}}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, typ := compile(t, test.program)
			if test.underlying {
				typ = typ.Underlying()
			}
			if got := uniqueName(typ); got != test.want {
				t.Fatalf("uniqueName(%v): got %q, want %q", typ, got, test.want)
			}
		})
	}
}

// TestExampleVersion is designed to make it hard to forget to update the
// codegen version when changes are made to the codegen API. Concretely,
// TestExampleVersion detects any changes to example/weaver_gen.go. If the file
// does change, the test fails and reminds you to update the codegen version.
//
// This is tedious, but codegen changes should be rare and should be
// increasingly rare as time goes on. Plus, forgetting to update the codegen
// version can be very annoying to users.
func TestExampleVersion(t *testing.T) {
	// Compute the SHA-256 hash of example/weaver_gen.go.
	f, err := os.Open("example/weaver_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprintf("%x", h.Sum(nil))

	// If weaver_gen.go has changed, the codegen version may need updating.
	const want = "149188524b7639a9470071875e68668c38b3145cd5f7d98559e1158da7761e4e"
	if got != want {
		t.Fatalf(`Unexpected SHA-256 hash of examples/weaver_gen.go: got %s, want %s. If this change is meaningful, REMEMBER TO UPDATE THE CODEGEN VERSION in runtime/version/version.go.`, got, want)
	}
}
