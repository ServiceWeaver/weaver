# Greet

This directory contains a Service Weaver application that demonstrates the fact
that component interfaces and component implementations can be in different
packages. This fact also allows you to change the implementation of a component
by changing which packages are linked into the binary.

The `greeter` package contains the `Greeter` interface:

```go
type Greeter interface {
    Greet(ctx context.Context, name string) (string, error)
}
```

The `english` package contains an implementation of the `Greeter` component:

```go
type english struct {
    weaver.Implements[greeter.Greeter]
}
```

The `mandarin` package contains a different implementation of the `Greeter`
component:

```go
type mandarin struct {
    weaver.Implements[greeter.Greeter]
}
```

`main.go` links in the Mandarin implementation:

```go
import _ "github.com/ServiceWeaver/weaver/examples/greet/mandarin"
```

If you run the application, you'll receive a mandarin greeting:

```console
$ go run .
你好，World！
```

You change `main.go` to instead link in the English implementation:

```go
import _ "github.com/ServiceWeaver/weaver/examples/greet/english"
```

This leads to an English greeting:

```console
$ go run .
Hello, World!
```
