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
	"syscall"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/retry"
)

func TestExamples(t *testing.T) {
	testCases := map[string]func(t *testing.T){
		"hello": httpRequestTestCase{
			url:          "http://localhost:12345/hello?name=something",
			expectedBody: "Hello, gnihtemos!\n",
		}.run,
		"collatz": httpRequestTestCase{
			url:          "http://127.0.0.1:9000?x=8",
			expectedBody: "8\n4\n2\n1\n",
		}.run,
		"reverser": httpRequestTestCase{
			url:          "http://127.0.0.1:9000/reverse?s=abcdef",
			expectedBody: "fedcba\n",
		}.run,
	}

	examples, err := os.ReadDir(".")
	isNilError(t, err)

	for _, example := range examples {
		if !example.IsDir() {
			continue
		}

		name := example.Name()
		t.Run(name, func(t *testing.T) {
			run := testCases[name]
			if run == nil {
				// TODO: make this t.Fatal once existing examples have tests
				t.Skip("no test case defined")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			t.Cleanup(cancel)

			chdir(t, name)
			cmd := startCmd(ctx, t, "go build .")
			if err := cmd.Wait(); err != nil {
				t.Fatalf("failed to build binary: %v", err)
			}

			t.Run("single", func(t *testing.T) {
				cmd := startCmd(ctx, t, "./"+name)
				t.Cleanup(terminateCmdAndWait(t, cmd))
				run(t)
			})

			t.Run("multi", func(t *testing.T) {
				cmd := startCmd(ctx, t, "../../cmd/weaver/weaver multi deploy weaver.toml")
				t.Cleanup(terminateCmdAndWait(t, cmd))
				run(t)
			})

			// TODO: other deployers?
		})
	}
}

type httpRequestTestCase struct {
	url          string
	expectedBody string
}

func (tc httpRequestTestCase) run(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	t.Cleanup(cancel)

	var resp *http.Response
	var err error
	for r := retry.Begin(); r.Continue(ctx); {
		resp, err = http.Get(tc.url)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("timeout waiting on listener, last error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected response code OK 200, got %v", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	isNilError(t, err)

	if actual := string(body); actual != tc.expectedBody {
		t.Fatalf("response body is %v, expected %v", actual, tc.expectedBody)
	}
}

func isNilError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func chdir(t *testing.T, path string) {
	t.Helper()
	orig, err := os.Getwd()
	isNilError(t, err)

	isNilError(t, os.Chdir(path))
	t.Cleanup(func() {
		isNilError(t, os.Chdir(orig))
	})
}

func startCmd(ctx context.Context, t *testing.T, c string) *exec.Cmd {
	t.Helper()
	cmd := exec.CommandContext(ctx, "sh", "-c", c)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	isNilError(t, cmd.Start())
	return cmd
}

func terminateCmdAndWait(t *testing.T, cmd *exec.Cmd) func() {
	return func() {
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatalf("failed to terminate process: %v", err)
		}
		if err := cmd.Wait(); err != nil {
			t.Fatalf("unexpected exit error: %v", err)
		}
	}
}
