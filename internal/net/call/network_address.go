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
	"strings"
)

// A NetworkAddress is a string of the form <network>://<address> (e.g.,
// "tcp://localhost:8000", "unix:///tmp/unix.sock").
type NetworkAddress string

// Split splits the network and address from a NetworkAddress. For example,
//
//	NetworkAddress("tcp://localhost:80").Split() // "tcp", "localhost:80"
//	NetworkAddress("unix://unix.sock").Split()   // "unix", "unix.sock"
func (na NetworkAddress) Split() (network string, address string, err error) {
	net, addr, ok := strings.Cut(string(na), "://")
	if !ok {
		return "", "", fmt.Errorf("%q does not have format <network>://<address>", na)
	}
	return net, addr, nil
}
