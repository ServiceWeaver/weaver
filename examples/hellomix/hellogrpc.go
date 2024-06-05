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
