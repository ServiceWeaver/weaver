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

package multi

import (
	"context"
	"fmt"
	"os"

	"github.com/ServiceWeaver/weaver"
	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// multiLogger overrides weaver.Logger component when running under the multi deployer.
type multiLogger weaver.Logger

type logger struct {
	weaver.Implements[multiLogger]
	logsDB  *logging.FileStore
	printer *logging.PrettyPrinter
}

var _ multiLogger = &logger{}

func newLogger(logsDB *logging.FileStore) *logger {
	return &logger{
		logsDB:  logsDB,
		printer: logging.NewPrettyPrinter(colors.Enabled()),
	}
}

func (l *logger) LogBatch(ctx context.Context, batch *protos.LogEntryBatch) error {
	for _, e := range batch.Entries {
		l.log(e)
	}
	return nil
}

func (l *logger) log(e *protos.LogEntry) {
	if !logging.IsSystemGenerated(e) {
		fmt.Fprintln(os.Stderr, l.printer.Format(e))
	}
	l.logsDB.Add(e)
}
