package main

import (
	"context"
	"fmt"

	"github.com/ServiceWeaver/weaver/internal/tool/multi"
	"github.com/ServiceWeaver/weaver/runtime/plugin"
	"github.com/ServiceWeaver/weaver/runtime/protos"
)

func main() {
	port := 10000
	plugins := &plugin.Plugins{
		GetListenerAddress: func(context.Context, *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error) {
			port++
			return &protos.GetListenerAddressReply{Address: fmt.Sprintf(":%d", port)}, nil
		},
		HandleLogEntry: func(ctx context.Context, entry *protos.LogEntry) error {
			fmt.Println(entry.Msg, entry.Attrs)
			return nil
		},
	}
	multi.Run(plugins)
}
