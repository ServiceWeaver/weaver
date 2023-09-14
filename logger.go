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

package weaver

import (
	"context"
	"fmt"
	"os"

	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// Logger is a component used by the Service Weaver implementation for saving
// log entries. This component is overridden by various deployers to customize
// how logs are stored. The default implementation writes log entries to os.Stderr
// and is hosted in every weavelet.
type Logger interface {
	LogBatch(context.Context, *protos.LogEntryBatch) error
}

type stderrLogger struct {
	Implements[Logger]
	pp *logging.PrettyPrinter
}

var _ Logger = &stderrLogger{}

// Init initializes the default Logger component.
func (logger *stderrLogger) Init(ctx context.Context) error {
	logger.pp = logging.NewPrettyPrinter(colors.Enabled())
	return nil
}

// LogBatch logs a list of entries.
func (logger *stderrLogger) LogBatch(ctx context.Context, batch *protos.LogEntryBatch) error {
	for _, entry := range batch.Entries {
		fmt.Fprintln(os.Stderr, logger.pp.Format(entry))
	}
	return nil
}
