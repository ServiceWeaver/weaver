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

package weaver

import (
	"github.com/ServiceWeaver/weaver/internal/logtype"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// Logger can be used to log messages at different levels (Debug, Info, Error).
// It can safely be used concurrently from multiple goroutines.
//
// A Service Weaver component implementation can get a logger for the
// component by calling the Logger() method on itself. Log messages
// written to such a logger will be automatically augmented with the
// component identity and location.
//
//	func (f *foo) SomeMethod() {
//	    logger := Logger()
//	    logger.Debug("This is a debug log!")
//	    logger.Info("This is an info log!")
//	    logger.Error("This is an error log!", io.EOF)
//	    return &foo{logger: logger}, nil
//	}
//
// You can also get a Logger from the root component returned by [weaver.Init].
//
//	func main() {
//	    root := weaver.Init(context.Background())
//	    logger := root.Logger()
//	    logger.Info("main started", "pid", os.Getpid())
//	    // ...
//	}
//
// # Attributes
//
// Loggers methods can be passed a list of key value pair attributes as trailing arguments.
// These attributes can be used for identifying and filtering log entries.
//
// An attribute is formed by two consecutive items in the attribute list,
// where the first item is a string containing the attribute name, and the
// second item is a value that can be converted to a string. If the
// preceding expectations are not met, the logged message will contain
// appropriate error markers.
//
// Examples:
//
//	var c weaver.Instance = ...
//	logger := c.Logger()
//
//	// No attributes added
//	logger.Debug("hello")
//
//	// Attributes added: {"host":"example.com", "port":12345}
//	logger.Info("ready", "host", "example.com", "port", 12345)
//
// # Pre-attached attributes
//
// If the same attributes will be used repeatedly, a logger can be created
// with those attributes pre-attached.
//
//	hostPortLogger := logger.With("host", "example.com", "port", 12345)
//
//	// Attributes added: {"host":"example.com", "port":12345, "method": "Put"}
//	hostPortLogger.Info("call", "method", "Put")
//
//	// Attributes added: {"host":"example.com", "port":12345, "method": "Get"}
//	hostPortLogger.Info("call", "method", "Get")
type Logger interface {
	// Debug logs an entry for debugging purposes that contains msg and attributes.
	Debug(msg string, attributes ...any)

	// Info logs an informational entry that contains msg and attributes.
	Info(msg string, attributes ...any)

	// Error logs an error entry that contains msg and attributes.
	// If err is not nil, it will be used as the value of a attribute
	// named "err".
	Error(msg string, err error, attributes ...any)

	// With returns a logger that will automatically add the
	// pre-specified attributes to all logged entries.
	With(attributes ...any) Logger
}

// Ensure that Logger implements logtype.Logger.
var _ logtype.Logger = Logger(nil)

// discardingLogger discards all logged entries.
type discardingLogger struct{}

var _ Logger = discardingLogger{}

func (d discardingLogger) Debug(string, ...any)        {}
func (d discardingLogger) Info(string, ...any)         {}
func (d discardingLogger) Error(string, error, ...any) {}
func (d discardingLogger) With(...any) Logger          { return d }

type attrLogger struct {
	logging.FuncLogger
}

// With implements [weaver.Logger].
func (l attrLogger) With(attrs ...any) Logger {
	result := l
	result.Opts.Attrs = logtype.AppendAttrs(l.Opts.Attrs, attrs)
	return result
}

func newAttrLogger(app, version, component, weavelet string, saver func(*protos.LogEntry)) Logger {
	return attrLogger{
		logging.FuncLogger{
			Opts: logging.Options{
				App:        app,
				Deployment: version,
				Component:  component,
				Weavelet:   weavelet,
			},
			Write: saver,
		},
	}
}
