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

package deployers

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"
)

// build runs "go build ." in the provided directories.
func build(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		cmd := exec.Command("go", "build", ".")
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
	}
}

// TestPipesDeployer tests the ./pipes deployer.
func TestPipesDeployer(t *testing.T) {
	build(t, "./pipes", "../../../examples/collatz")
	cmd := exec.Command("./pipes/pipes", "../../../examples/collatz/collatz")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}

// TestSingleDeployer tests the ./single deployer.
func TestSingleDeployer(t *testing.T) {
	build(t, "./single", "../../../examples/collatz")
	deployCollatz(t, "./single/single")
}

// TestMultiDeployer tests the ./multi deployer.
func TestMultiDeployer(t *testing.T) {
	build(t, "./multi", "../../../examples/collatz")
	deployCollatz(t, "./multi/multi")
}

// deployCollatz deploys collatz with the provided deployer binary.
func deployCollatz(t *testing.T, deployer string) {
	// Deploy collatz.
	cmd := exec.Command(deployer, "../../../examples/collatz/collatz")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()

	// Parse the listener address from the logs (yes, this is janky).
	addr := ""
	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Weavelet listening on ") {
			addr, _ = strings.CutPrefix(line, "Weavelet listening on ")
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	// Curl the listener.
	url := fmt.Sprintf("http://%s?x=10", addr)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	got := string(bytes)
	const want = "10\n5\n16\n8\n4\n2\n1\n"
	if got != want {
		t.Fatalf("curl %s: got %q, want %q", url, got, want)
	}
}
