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

package logging

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

func TestTestLogger(t *testing.T) {
	// Test plan: Launch a goroutine that continues to write to a TestLogger
	// after the test ends. The logger should stop logging when the test ends.
	t.Run("sub", func(t *testing.T) {
		logger := NewTestLogger(t)
		go func() {
			for {
				logger.Debug("Ping")
			}
		}()
		// Give the logger a chance to log something.
		time.Sleep(1 * time.Millisecond)
	})
	// Allow the goroutine to keep running, even though the test has finished.
	time.Sleep(50 * time.Millisecond)
}

func BenchmarkMakeEntry(b *testing.B) {
	opt := Options{
		App:        "app",
		Deployment: "dep",
		Component:  "comp",
		Weavelet:   "wlet",
	}
	for i := 0; i < b.N; i++ {
		e := makeEntry("info", "test", nil, 0, opt)
		proto.Marshal(e)
	}
}
