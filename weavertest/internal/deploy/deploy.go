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

package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ServiceWeaver/weaver"
	"github.com/google/uuid"
)

//go:generate ../../../cmd/weaver/weaver generate

type Started interface {
	MarkStarted(_ context.Context, dir string) error
}

// started is a Service Weaver component that can mark itself as started.
type started struct {
	weaver.Implements[Started]
	id uuid.UUID
}

func (d *started) Init(context.Context) error {
	d.id = uuid.New()
	return nil
}

// MarkStarted writes a unique file to the provided directory with suffix
// "started". You can count the number of started components by counting the
// number of "*.started" files.
func (d *started) MarkStarted(_ context.Context, dir string) error {
	filename := filepath.Join(dir, fmt.Sprintf("%s.started", d.id))
	return os.WriteFile(filename, []byte{}, 0600)
}

type Widget interface {
	Use(ctx context.Context, dir string) error
}

// widget is a Service Weaver component that deploys started Service Weaver components.
type widget struct {
	weaver.Implements[Widget]
	started weaver.Ref[Started]
}

// Use calls MarkStarted on every replica.
func (w *widget) Use(ctx context.Context, dir string) error {
	// Print to stderr. This tests that weavertest output is captured correctly.
	fmt.Fprintf(os.Stderr, "widget.Use(%q)\n", dir)

	// There are n started replicas, and we want to call MarkStarted on all
	// of them. We don't have a way to call MarkStarted on a specific replica,
	// so we call MarkStarted a bunch of times. This makes it very likely that
	// every replica receives at least one call to MarkStarted.
	for i := 0; i < 1000; i++ {
		if err := w.started.Get().MarkStarted(ctx, dir); err != nil {
			return err
		}
	}
	return nil
}
