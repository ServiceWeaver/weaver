# Fakes

This directory demonstrates how to fake a component in a weavertest. `clock.go`
contains an implementation of a `Clock` component. `clock_test.go` tests the
`Clock` component, but substitutes a fake for the real implementation. Run `go
test` to run the test.
