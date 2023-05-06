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

package diverge

import (
	"context"
	"fmt"

	"github.com/ServiceWeaver/weaver"
)

type IntError struct {
	X int
}

func (e IntError) Is(target error) bool {
	_, ok := target.(IntError)
	return ok
}

func (e IntError) Error() string {
	return fmt.Sprintf("IntError(%d)", e.X)
}

type Errer interface {
	Err(context.Context, int) error
}

type errer struct{ weaver.Implements[Errer] }

func (e *errer) Err(_ context.Context, x int) error {
	return IntError{x}
}
