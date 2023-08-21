// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package examples

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/retry"
)

type test struct {
	url  string // url to GET
	want string // expected response
}

func TestExamples(t *testing.T) {
	// Build the weaver binary.
	cmd := exec.Command("go", "build")
	cmd.Dir = "../cmd/weaver"
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	testCases := map[string]test{
		// TODO(mwhittaker): Add helloworld example.
		"hello": {
			url:  "http://localhost:12345/hello?name=something",
			want: "Hello, gnihtemos!",
		},
		"collatz": {
			url:  "http://127.0.0.1:9000?x=8",
			want: "8\n4\n2\n1",
		},
		"reverser": {
			url:  "http://127.0.0.1:9000/reverse?s=abcdef",
			want: "fedcba",
		},
		"factors": {
			url:  "http://127.0.0.1:9000?x=10",
			want: "[1 2 5 10]",
		},
	}

	examples, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	for _, example := range examples {
		if !example.IsDir() {
			continue
		}

		name := example.Name()
		t.Run(name, func(t *testing.T) {
			test, ok := testCases[name]
			if !ok {
				// TODO: make this t.Fatal once existing examples have tests
				t.Skip("no test case defined")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			t.Cleanup(cancel)

			// Build the example binary.
			chdir(t, name)
			cmd := startCmd(ctx, t, nil, "go", "build", ".")
			if err := cmd.Wait(); err != nil {
				t.Fatalf("failed to build binary: %v", err)
			}

			// Run the application directly.
			t.Run("single", func(t *testing.T) {
				env := []string{"SERVICEWEAVER_CONFIG=weaver.toml"}
				cmd := startCmd(ctx, t, env, "./"+name)
				t.Cleanup(terminateCmdAndWait(t, cmd))
				run(t, test)
			})

			// "weaver single deploy" the application.
			t.Run("weaver-single", func(t *testing.T) {
				cmd := startCmd(ctx, t, nil, "../../cmd/weaver/weaver", "single", "deploy", "weaver.toml")
				t.Cleanup(terminateCmdAndWait(t, cmd))
				run(t, test)
			})

			// "weaver multi deploy" the application.
			t.Run("weaver-multi", func(t *testing.T) {
				cmd := startCmd(ctx, t, nil, "../../cmd/weaver/weaver", "multi", "deploy", "weaver.toml")
				t.Cleanup(terminateCmdAndWait(t, cmd))
				run(t, test)
			})

			// TODO: other deployers?
		})
	}
}

func run(t *testing.T, test test) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	t.Cleanup(cancel)

	// Send a GET request to the endpoint, retrying on error.
	client := http.DefaultClient
	var resp *http.Response
	var err error
	for r := retry.Begin(); r.Continue(ctx); {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, test.url, nil)
		if reqErr != nil {
			t.Fatal(reqErr)
		}

		resp, err = client.Do(req)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("timeout waiting on listener, last error: %v", err)
	}
	defer resp.Body.Close()

	// Check that the GET response is as expected.
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected response code OK 200, got %v", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if actual := string(body); !strings.Contains(actual, test.want) {
		t.Fatalf("response body is %v, expected %v", actual, test.want)
	}
}

// chdir cd's to the provided path, cd'ing back to the current working
// directory when the test finishes.
func chdir(t *testing.T, path string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatal(err)
		}
	})
}

func startCmd(ctx context.Context, t *testing.T, env []string, name string, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.CommandContext(ctx, name, args...)
	if testing.Verbose() {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = append(os.Environ(), env...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return cmd
}

func terminateCmdAndWait(t *testing.T, cmd *exec.Cmd) func() {
	return func() {
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatalf("failed to terminate process: %v", err)
		}
		if err := cmd.Wait(); err != nil {
			t.Logf("exit error: %v", err)
		}
	}
}
