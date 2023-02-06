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

package pipe

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// TestPipe runs itself as a subprocess. When the "echo" argument is
	// provided, the test echos to file descriptor 3 and exits.
	flag.Parse()
	if flag.Arg(0) == "echo" {
		if err := echo(); err != nil {
			panic(err)
		}
		return
	}

	os.Exit(m.Run())
}

func TestRWPipe(t *testing.T) {
	// Run the command.
	ex, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := CommandContext(context.Background(), ex, "echo")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	var w io.WriteCloser
	{
		var fd int
		if fd, w, err = cmd.WPipe(); err != nil {
			t.Fatal(err)
		} else if fd != 3 {
			t.Fatalf("bad fd: got %d, want 3", fd)
		}
	}
	var r io.ReadCloser
	{
		var fd int
		if fd, r, err = cmd.RPipe(); err != nil {
			t.Fatal(err)
		} else if fd != 4 {
			t.Fatalf("bad fd: got %d, want 4", fd)
		}
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	const msg = "Hello, World!"
	// Write the message.
	if _, err := fmt.Fprint(w, msg); err != nil {
		t.Fatal(err)
	}
	w.Close() // signal EOF

	// Read the echo.
	bytes, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := string(bytes), msg; got != want {
		t.Fatalf("bad echo value: got %q, want %q", got, want)
	}

	// Wait for the command to terminate.
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

// echo reads a msg from file descriptor 3 and writes it to file descriptor 4.
func echo() error {
	r := os.NewFile(3, "/proc/self/fd/3")
	if r == nil {
		return fmt.Errorf("unable to open fd 3")
	}
	w := os.NewFile(4, "/proc/self/fd/4")
	if r == nil {
		return fmt.Errorf("unable to open fd 4")
	}
	msg, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, string(msg)); err != nil {
		return err
	}
	return nil
}
