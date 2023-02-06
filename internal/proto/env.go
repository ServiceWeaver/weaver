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

package proto

import (
	"encoding/base64"

	"google.golang.org/protobuf/proto"
)

// ToEnv converts msg to a string suitable for storing in the process's
// OS environment (i.e., os.Environ()).
func ToEnv(msg proto.Message) (string, error) {
	data, err := proto.Marshal(msg)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// FromEnv fills msg from a string created by ToEnv.
func FromEnv(in string, msg proto.Message) error {
	data, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		return nil
	}
	return proto.Unmarshal(data, msg)
}
