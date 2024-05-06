package multigrpc

import (
	"path/filepath"

	"github.com/ServiceWeaver/weaver/runtime"
	"github.com/ServiceWeaver/weaver/runtime/tool"
)

var (
	// The directory and files where "weaver multigrpc" stores logs.
	logDir = filepath.Join(runtime.LogsDir(), "multigrpc")

	Commands = map[string]*tool.Command{
		"deploy": &deployCmd,
	}
)
