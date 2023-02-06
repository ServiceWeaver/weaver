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

package protomsg

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"google.golang.org/protobuf/proto"
	"github.com/ServiceWeaver/weaver/runtime/codegen"
)

// CallArgs holds arguments for the Call method.
type CallArgs struct {
	Client  *http.Client
	Host    string
	Addr    string
	URLPath string
	Request proto.Message
	Reply   proto.Message
}

// Call invokes an HTTP method on the given address/path combo, passing it a
// serialized request and parsing its response into reply.
// If called with nil request, a GET HTTP method is issued; otherwise,
// a POST HTTP method is issued.
// If reply is nil, the response is discarded.
func Call(ctx context.Context, args CallArgs) error {
	var out *http.Response
	hasError := func(r *http.Response) bool {
		return r != nil && r.StatusCode != http.StatusOK
	}
	getError := func(r *http.Response) error {
		if r == nil || r.StatusCode == http.StatusOK {
			return nil
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			return err
		}
		if len(b) == 0 {
			return fmt.Errorf("HTTP status %d", r.StatusCode)
		}
		return fmt.Errorf("HTTP status %d: %s", r.StatusCode, b)
	}
	url := fmt.Sprintf("%s%s", args.Addr, args.URLPath)
	method := "POST"
	if args.Request == nil {
		method = "GET"
	}
	var in []byte
	if args.Request != nil {
		var err error
		if in, err = toWire(args.Request); err != nil {
			return fmt.Errorf("bad request for %s: %w", url, err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(in))
	if err != nil {
		return fmt.Errorf("cannot create an HTTP request: %w", err)
	}
	if args.Host != "" {
		req.Host = args.Host
	}

	out, err = args.Client.Do(req)
	if err != nil {
		return err
	}
	defer out.Body.Close()
	if hasError(out) {
		return fmt.Errorf("cannot call %q: %w", url, getError(out))
	}
	if args.Reply == nil {
		return nil
	}
	res, err := io.ReadAll(out.Body)
	if err != nil {
		return fmt.Errorf("bad response from %s: %w", url, err)
	}
	if err := fromWire(res, args.Reply); err != nil {
		return fmt.Errorf("bad result from %s: %w", url, err)
	}
	return nil
}

// toWire converts the given messages to a byte slice that is suitable for
// sending over the network.
func toWire(msgs ...proto.Message) (data []byte, err error) {
	// Catch and return any panics detected during the encoding.
	defer func() { err = codegen.CatchPanics(recover()) }()
	enc := codegen.NewEncoder()
	for _, msg := range msgs {
		b, err := proto.Marshal(msg)
		if err != nil {
			return nil, err
		}
		enc.Bytes(b)
	}
	data = enc.Data()
	return
}

// fromWire fills msgs from a byte slice created by ToWire.
func fromWire(in []byte, msgs ...proto.Message) (err error) {
	// Catch and return any panics detected during the decoding.
	defer func() { err = codegen.CatchPanics(recover()) }()
	dec := codegen.NewDecoder(in)
	for _, msg := range msgs {
		b := dec.Bytes()
		err = proto.Unmarshal(b, msg)
		if err != nil {
			return
		}
	}
	return
}
