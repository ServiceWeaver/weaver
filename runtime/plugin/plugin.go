package plugin

import (
	"context"

	"github.com/ServiceWeaver/weaver/runtime/protos"
)

type Plugins struct {
	GetListenerAddress func(context.Context, *protos.GetListenerAddressRequest) (*protos.GetListenerAddressReply, error)
	HandleLogEntry     func(context.Context, *protos.LogEntry) error
	HandleTraceSpans   func(context.Context, *protos.TraceSpans) error
}
