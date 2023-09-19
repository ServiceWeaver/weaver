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

package sim

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver"
)

// fakeRegister is a fake implementation of the register component.
type fakeRegister struct {
	mu sync.Mutex
	s  string
}

func (f *fakeRegister) Append(_ context.Context, s string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.s = f.s + s
	return f.s, nil
}

func (f *fakeRegister) Clear(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.s = ""
	return nil
}

// See TestRegisterWorkload.
type registerWorkload struct {
	reg  weaver.Ref[register]
	fake fakeRegister
}

func (w *registerWorkload) Init(r Registrar) error {
	alphabet := strings.Split("abcdefghijklmnopqrstuvwxyz", "")
	r.RegisterFake(Fake[register](&w.fake))
	r.RegisterGenerators("Append", OneOf(alphabet...))
	r.RegisterGenerators("Clear")
	return nil
}

func (w *registerWorkload) Append(ctx context.Context, s string) error {
	s, err := w.reg.Get().Append(ctx, s)
	if err != nil {
		return nil
	}
	if s == "hello" {
		return fmt.Errorf("hello")
	}
	return nil
}

func (w *registerWorkload) Clear(ctx context.Context) error {
	w.reg.Get().Clear(ctx)
	return nil
}

func TestRegisterWorkload(t *testing.T) {
	// TestRegisterWorkload showcases the importance of test case minimization.
	// The workload randomly appends letters to a string, failing if the string
	// "hello" is ever formed. The odds of picking five random letters and
	// forming the string "hello" is (1/26)^5, quite unlikely. The odds of
	// picking 100 random letters and having a subsequence of them form "hello"
	// is much more likely.
	//
	// Thus, when we do find an execution that forms "hello", it is probably a
	// long execution that appends a lot of garbage letters, eventually clears
	// the string, and then appends "hello". These long executions can benefit
	// from minimization.
	//
	// TODO(mwhittaker): Consider what happens when we don't have a Clear
	// method. In this case, the simulator may run forever without finding a
	// "hello" sequence. We want to make sure our search algorithm is smart
	// enough to eventually find "hello".
	s := New(t, &registerWorkload{}, Options{})
	if results := s.Run(1 * time.Second); results.Err != nil {
		t.Log(results.Mermaid())
		t.Log(results.Err)
	}
}
