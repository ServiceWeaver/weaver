package conn

import "io"

type contextKeyT struct{}

// ContextKey is used to store an IO used by tests to start an Envelope without
// that envelope starting a weavelet process.
var ContextKey = contextKeyT{}

type IO struct {
	Reader io.ReadCloser
	Writer io.WriteCloser
}
