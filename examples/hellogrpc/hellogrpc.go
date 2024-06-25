// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type HelloGrpcSVL struct {
	HelloFromSVLServer
	mtvHandle HelloFromMTVClient
}

type HelloGrpcMTV struct {
	HelloFromMTVServer
}

func (h *HelloGrpcSVL) Hello(ctx context.Context, req *HelloRequest) (*HelloResponse, error) {
	d, err := h.mtvHandle.GetDate(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	t := time.Unix(d.GetSeconds(), int64(d.GetNanos())).Local()
	return &HelloResponse{
		Response: fmt.Sprintf("[SVL][%d] Hello, %s at %v!", os.Getpid(), req.Request, t),
	}, nil
}

func (h *HelloGrpcMTV) Hello(ctx context.Context, req *HelloRequest) (*HelloResponse, error) {
	d, err := h.GetDate(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	t := time.Unix(d.GetSeconds(), int64(d.GetNanos())).Local()
	return &HelloResponse{
		Response: fmt.Sprintf("[MTV][%d] Hello, %s at %v!", os.Getpid(), req.Request, t),
	}, nil
}

func (h *HelloGrpcMTV) GetDate(context.Context, *emptypb.Empty) (*timestamppb.Timestamp, error) {
	now := time.Now()
	return &timestamppb.Timestamp{
		Seconds: now.Unix(),
		Nanos:   int32(now.Nanosecond()),
	}, nil
}
