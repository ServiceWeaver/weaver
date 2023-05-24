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

package protos

import (
	"context"
	"testing"

	"github.com/ServiceWeaver/weaver/weavertest"
)

func TestPingPong(t *testing.T) {
	ctx := context.Background()
	weavertest.Multi.Test(t, func(t *testing.T, pingponger PingPonger) {
		pong, err := pingponger.Ping(ctx, &Ping{Id: 42})
		if err != nil {
			t.Fatal(err)
		}
		if pong.Id != 42 {
			t.Fatalf("bad pong: got %v, want %v", pong.Id, 42)
		}
	})
}
