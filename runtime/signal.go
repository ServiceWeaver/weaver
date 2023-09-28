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

package runtime

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var (
	mu  sync.Mutex
	fns []func()
)

// OnExitSignal arranges to run fn() when a signal that might cause the
// process to exit is delivered.
func OnExitSignal(fn func()) {
	mu.Lock()
	defer mu.Unlock()
	fns = append(fns, fn)
	if len(fns) != 1 {
		return
	}

	// Install handler on first call.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sig
		mu.Lock()
		defer mu.Unlock()
		// Run cleanup functions in reverse order to match usual destruction order.
		for i := len(fns) - 1; i >= 0; i-- {
			fns[i]()
		}
		if num, ok := s.(syscall.Signal); ok {
			os.Exit(128 + int(num))
		}
		os.Exit(1)
	}()
}
