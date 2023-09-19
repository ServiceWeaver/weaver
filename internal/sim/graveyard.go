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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// # Simulator Graveyard
//
// When the simulator runs a failed execution, it persists the failing inputs
// to disk. The collection of saved failing inputs is called a *graveyard*, and
// each individual entry is called a *graveyard entry*. When a simulator is
// created, the first thing it does is load and re-simulate all graveyard
// entries.
//
// We borrow the design of go's fuzzing library's corpus with only minor
// changes [1]. When a simulator runs as part of a test named TestFoo, it
// stores its graveyard entries in testdata/sim/TestFoo. Every graveyard entry
// is a file containing a JSON-encoded GraveyardEntry. Filenames are derived
// from the hash of the contents of the graveyard entry. Here's an example
// testdata directory:
//
//     testdata/
//     └── sim
//         ├── TestCancelledSimulation
//         │   └── a52f5ec5f94e674d.json
//         ├── TestSimulateGraveyardEntries
//         │   ├── 2bfe847328319dae.json
//         │   └── a52f5ec5f94e674d.json
//         └── TestUnsuccessfulSimulation
//             ├── 2bfe847328319dae.json
//             └── a52f5ec5f94e674d.json
//
// As with go's fuzzing library, graveyard entries are never garbage collected.
// Users are responsible for manually deleting graveyard entries when
// appropriate.
//
// ## Determinism
//
// Note that a simulation is determined by its inputs as well as the code being
// simulated. A GraveyardEntry captures the inputs to a simulation, but not the
// code being simulated. For this reason, the way a GraveyardEntry is simulated
// will change as a user changes their code. This means that a GraveyardEntry
// may yield simulations that differ from its original execution. That is okay.
//
// TODO(mwhittaker): Right now, a GraveyardEntry only contains a seed. This
// means that even small changes in the user's code can lead to wildly
// different simulations. In the future, we may want to persist the execution
// itself and try to recreate the execution as faithfully as possible. This
// kind of replay will also likely be needed for minimization.
//
// ## Visualization
//
// TODO(mwhittaker): Store the history of a simulation in a graveyard entry so
// failing executions can be visualized.
//
// ## Versioning
//
// The contents of a GraveyardEntry may change over time. To address this,
// every GraveyardEntry contains a version. If a simulator loads a
// GraveyardEntry with a different version than what it is expecting, the
// GraveyardEntry is ignored.
//
// TODO(mwhittaker): Switch to using protobufs?
//
// [1]: https://go.dev/security/fuzz

// The current version of the GraveyardEntry API.
const version = 1

// graveyardEntry is a set of failing inputs persisted for later execution.
type graveyardEntry struct {
	Version     int     `json:"version"`
	Seed        int64   `json:"seed"`
	NumReplicas int     `json:"num_replicas"`
	NumOps      int     `json:"num_ops"`
	FailureRate float64 `json:"failure_rate"`
	YieldRate   float64 `json:"yield_rate"`
}

// readGraveyard reads all the graveyard entries stored in the provided directory.
func readGraveyard(dir string) ([]graveyardEntry, error) {
	// This code borrows from https://cs.opensource.google/go/go/+/master:src/internal/fuzz/fuzz.go;drc=14ab998f95b53baa6e336c598b0f34e319cc9717.
	files, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil // No graveyard to read
	} else if err != nil {
		return nil, fmt.Errorf("read graveyard %q: %w", dir, err)
	}

	var graveyard []graveyardEntry
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			// The file is not a graveyard entry file.
			continue
		}

		filename := filepath.Join(dir, file.Name())
		data, err := os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("read graveyard entry file %q: %w", filename, err)
		}
		var entry graveyardEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return nil, fmt.Errorf("unmarshal graveyard entry file %q: %w", filename, err)
		}
		if entry.Version != version {
			continue
		}
		graveyard = append(graveyard, entry)
	}
	return graveyard, nil

}

// writeGraveyardEntry writes a graveyard entry to the provided directory and
// returns the filename of the written file.
func writeGraveyardEntry(dir string, entry graveyardEntry) (string, error) {
	// This code borrows from https://cs.opensource.google/go/go/+/master:src/internal/fuzz/fuzz.go;drc=14ab998f95b53baa6e336c598b0f34e319cc9717.
	data, err := json.MarshalIndent(entry, "", "    ")
	if err != nil {
		return "", fmt.Errorf("marshal graveyard entry: %w", err)
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(data))[:16]
	name := fmt.Sprintf("%s.json", sum)
	filename := filepath.Join(dir, name)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return "", err
	}
	// TODO(mwhittaker): Atomically write files using renames.
	if err := os.WriteFile(filename, data, 0666); err != nil {
		os.Remove(filename) // remove partially written file
		return "", fmt.Errorf("write graveyard entry file %q: %w", filename, err)
	}
	return filename, nil
}
