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

package call

import (
	"fmt"

	"github.com/ServiceWeaver/weaver/runtime/codegen"
)

type transportError int

const (
	// CommunicationError is the type of the error returned by a call when some
	// communication error is encountered, typically a process or network
	// error. Check for it via errors.Is(call.CommunicationError).
	CommunicationError transportError = iota

	// Unreachable is the type of the error returned by a call when every
	// server is unreachable. Check for it via errors.Is(call.Unreachable).
	Unreachable

	// TODO: Decide what error most applications will want to check for. We may
	// need to combine CommunicationError and Unreachable. We may also want to
	// make errors.Is(CommunicationError) return true for both types of errors.
	// This is doable by defining transportError.Is (https://pkg.go.dev/errors#Is)
)

// Error implements the error interface.
func (e transportError) Error() string {
	switch e {
	case CommunicationError:
		return "communication error"
	case Unreachable:
		return "unreachable"
	default:
		return fmt.Sprintf("unknown error %d", e)
	}
}

func encodeError(err error) []byte {
	// TODO(sanjay): There is a tiny risk that encoding the error will fail if
	// we end up generating a string whose length does not fit in four bytes.
	// We chose to ignore that possibility since it is rare and it is hard
	// to figure out what to do in that case (we could attempt to encode another
	// error, but that itself could perhaps fail). A future change should fix
	// this problem.
	e := codegen.NewEncoder()
	e.Error(err)
	return e.Data()
}

func decodeError(msg []byte) (err error, ok bool) {
	defer func() {
		if x := codegen.CatchPanics(recover()); x != nil {
			err = x
		}
	}()
	dec := codegen.NewDecoder(msg)
	err = dec.Error()
	ok = true
	return
}
