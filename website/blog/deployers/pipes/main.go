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

// Package main implements a trivial deployer that demonstrates the raw
// envelope-weavelet API. See https://serviceweaver.dev/blog/deployers.html for
// corresponding blog post.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/ServiceWeaver/weaver/runtime/protomsg"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/prototext"
)

// Usage: ./pipes <service weaver binary>
func main() {
	flag.Parse()
	if err := run(context.Background(), flag.Arg(0)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, binary string) error {
	// Step 1. Run the binary and establish the pipes between the envelope and
	// the weavelet.
	//
	//              envelope               weavelet
	//             ┌──────────────────┐   ┌───────────────────┐
	//             │ envelopeWriter --│---│-> weaveletReader  │
	//             │ envelopeReader <-│---│-- weaveletWriter  │
	//             └──────────────────┘   └───────────────────┘
	weaveletReader, envelopeWriter, err := os.Pipe()
	if err != nil {
		return err
	}
	envelopeReader, weaveletWriter, err := os.Pipe()
	if err != nil {
		return err
	}

	// ExtraFiles file descriptors begin at 3 because descriptors 0, 1, and 2
	// are reserved for stdin, stdout, and stderr. See
	// https://pkg.go.dev/os/exec#Cmd for details.
	cmd := exec.Command(binary)
	cmd.ExtraFiles = []*os.File{weaveletReader, weaveletWriter}
	cmd.Env = append(cmd.Environ(), "ENVELOPE_TO_WEAVELET_FD=3", "WEAVELET_TO_ENVELOPE_FD=4")
	if err := cmd.Start(); err != nil {
		return err
	}

	// Step 2. Send an EnvelopeInfo to the weavelet.
	info := &protos.EnvelopeMsg{
		EnvelopeInfo: &protos.EnvelopeInfo{
			App:             "app",               // the application name
			DeploymentId:    uuid.New().String(), // the deployment id
			Id:              uuid.New().String(), // the weavelet id
			RunMain:         true,                // should the weavelet run main?
			InternalAddress: "localhost:0",       // internal address of the weavelet
		},
	}
	if err := protomsg.Write(envelopeWriter, info); err != nil {
		return err
	}

	// Step 3. Receive a WeaveletInfo from the weavelet.
	var reply protos.WeaveletMsg
	if err := protomsg.Read(envelopeReader, &reply); err != nil {
		return err
	}
	fmt.Println(prototext.Format(&reply))

	// Step 4. Send a GetHealth RPC to the weavelet.
	req := &protos.EnvelopeMsg{
		Id:               42,
		GetHealthRequest: &protos.GetHealthRequest{},
	}
	if err := protomsg.Write(envelopeWriter, req); err != nil {
		return err
	}

	// Step 5. Receive a reply to the GetHealth RPC, ignoring other messages.
	for {
		var reply protos.WeaveletMsg
		if err := protomsg.Read(envelopeReader, &reply); err != nil {
			return err
		}
		if reply.Id == -42 {
			fmt.Println(prototext.Format(&reply))
			break
		}
	}
	return nil
}
