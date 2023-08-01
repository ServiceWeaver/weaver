# Using Advanced Go Features to Detect Stale Code

<div class="blog-author">Michael Whittaker</div>
<div class="blog-date">August 9, 2023</div>

Service Weaver is a programming framework for writing distributed systems in Go.
Service Weaver programs are composed of actor-like entities called
[**components**][components], which are defined using native Go constructs. For
example, we can define the interface and implementation of a `Calculator`
component using an `interface` and `struct` respectively:

```go
// Calculator component interface.
type Calculator interface {
    Add(context.Context, int, int) (int, error)
}

// Calculator component implementation.
type calc struct {
    weaver.Implements[Calculator]
}

func (*calc) Add(_ context.Context, x, y int) (int, error) {
    return x + y, nil
}
```

One component can call another component's methods, even if the two components
are running on different machines. With Service Weaver, you don't have to
implement these remote procedure calls yourself. Instead, Service Weaver
provides a **code generator**, `weaver generate`, that generates the code needed
to execute method calls as remote procedure calls. `weaver generate` writes the
generated code to a file called `weaver_gen.go`, which is compiled along with
the rest of your application.

```shell
$ weaver generate . # writes weaver_gen.go
$ go build .        # builds everything, including weaver_gen.go
```

Whenever you make a significant change to a Service Weaver app, you have to
re-run `weaver generate`. For example, if we add a `Subtract` method to the
`Calculator` component, we need to re-run `weaver generate` to generate the code
that executes `Subtract` calls as remote procedure calls.

Unfortunately, it's easy to forget to run `weaver generate` and to try and build
an application with a stale `weaver_gen.go`. We want to detect this as early as
possible, preferably at compile time rather than runtime. In this blog post, we
describe some of advanced ways we use Go to detect, at compile time, when you
forget to run `weaver generate`.

## Component Interfaces

You may forget to run `weaver generate` after adding, removing, or changing a
method in a component's interface (e.g., `Calculator`).

```diff
 type Calculator interface {
    Add(context.Context, int, int) (int, error)
+   Subtract(context.Context, int, int) (int, error)
 }
```

For every component interface `I`, `weaver generate` generates a pair of structs
to execute `I`'s methods as remote procedure calls. One of the structs acts as a
client, and the other acts as a server. For the `Calculator` component, for
example, `weaver generate` generates

1. a client struct called `calc_client_stub` and
2. a server struct called `calc_server_stub`.

The client struct implements interface `I`, and `weaver generate` generates code
to check this at compile time:

```go
// Check that calc_client_stub implements the Calculator interface.
var _ Calculator = (*calc_client_stub)(nil)
```

If you add or change a method in a component's interface but forget to re-run
`weaver generate`, this check will fail at compile time, as `weaver generate`
hasn't had an opportunity to generate an implementation of the new or changed
method.

The server stub receives remote procedure calls and dispatches them to a local
instance of the component implementation (`calc` in this example). Thus, if you
remove a method from a component's interface, the server struct will be calling
a method that no longer exists, and your code will fail to build.

## Component Implementations

You may forget to run `weaver generate` after adding, removing, or changing a
`weaver.Implements[T]` embedded inside a component implementation struct (e.g.,
`calc`).

```diff
 type calc struct {
-     weaver.Implements[Calculator]
 }
```

To detect this, `weaver.Implements[T]` implements an unexported `implements(T)`
method:

```go
type Implements[T any] struct { ... }
func (Implements[T]) implements(T) {}
```

Any struct that embeds `weaver.Implements[T]` inherits this `implements(T)`
method:

```go
type calc struct {
    weaver.Implements[Calculator]
}

// calc inherits the implements method from the embedded weaver.Implements.
var _ func(Calculator) = calc{}.implements
```

We then introduce an `InstanceOf[T]` interface:

```go
type InstanceOf[T any] interface {
    implements(T)
}
```

Because `implements(T)` is unexported, only structs that embed
`weaver.Implements[T]` will implement the `weaver.InstanceOf[T]` interface.
`weaver generate` generates code to check this at compile time:

```go
// Check that calc embeds weaver.InstanceOf[Calculator].
var _ weaver.InstanceOf[Calculator] = (*calc)(nil)
```

If you remove or change a `weaver.Implements[T]` embedded inside a component
implementation struct, the previous check will fail to compile. Unfortunately,
we do not currently have a way to detect when you *add* `weaver.Implements[T]`
to a struct but forget to re-run `weaver generate`. In this case, the
application will panic immediately when run.

## Serializable Types

With Service Weaver, basic types like ints, bools, strings, pointers, slices,
and maps are [serializable by default][serializability], but structs are not.
However, they can trivially be made serializable by embedding
`weaver.AutoMarshal`.

```go
type Pair struct {
    weaver.AutoMarshal
    x int
    y int
}
```

When `weaver generate` encounters a struct with an embedded
`weaver.AutoMarshal`, like `Pair` above, it generates methods to encode and
decode instances of the struct.


```go
func (p *Pair) WeaverMarshal(enc *codegen.Encoder) {
    enc.Int64(p.x)
    enc.Int64(p.y)
}

func (p *Pair) WeaverUnmarshal(dec *codegen.Decoder) {
    p.x = dec.Int64()
    p.y = dec.Int64()
}
```

You may forget to run `weaver generate` after adding, removing, or changing
fields inside a struct that embeds `weaver.AutoMarshal`.

```diff
type Pair struct {
    weaver.AutoMarshal
-   x int
    y int
+   z int
}
```

To detect this, `weaver generate` copies the definition of the struct `S` into
`weaver_gen.go`, instantiates it, and assigns it to a variable of type `S`. For
`Pair`, that looks like this:

```go
var _ Pair = struct{
    weaver.AutoMarshal
    x int
    y int
}{}
```

If you change the definition of `Pair` in any way but forget re-run `weaver
generate`, this assignment fails to build.

## Routers

Components are replicated, and by default, method calls are routed to random
component replicas. Service Weaver allows you to override this behavior with a
[**router**][routing] that specifies exactly how to route method calls.
Consider a key-value cache component as an example:

```go
// Cache component interface.
type Cache interface {
    Get(ctx context.Context, key string) (string, error)
    Put(ctx context.Context, key, value string) error
}

// Cache component implementation.
type cache struct {
    weaver.Implements[Cache]
    ...
}
func (*cache) Get(ctx context.Context, key string) (string, error) { ... }
func (*cache) Put(ctx context.Context, key, value string) error { ... }
```

To route method calls to the `Cache` component, we define a router struct that
returns a routing key for every `Cache` method. Here, the router uses the `key`
argument passed to the `Get` and `Put` methods:

```go
type router struct{}
func (router) Get(_ context.Context, key string) string { return key }
func (router) Put(_ context.Context, key, value string) string { return key }
```

Next, we embed `weaver.WithRouter[router]` in the `cache` struct:

```go
type cache struct {
    weaver.Implements[Cache]
    weaver.WithRouter[router]
    ...
}
```

With all this in place, `Cache` method calls with the same key will be routed to
the same replica of the `Cache` component.

You may forget to run `weaver generate` after adding, removing, or changing an
embedded `weaver.WithRouter[T]`.

```diff
 type cache struct {
     weaver.Implements[Cache]
-    weaver.WithRouter[router]
 }
```

To detect this, we introduce a `weaver.RoutedBy[T]` interface and
`weaver.Unrouted` interface.  If a component struct embeds
`weaver.WithRouter[T]`, then it implements `weaver.RoutedBy[T]`. All other
component structs implement `weaver.Unrouted`.

```go
// Component A is routed.
type a struct {
    weaver.Implements[A]
    weaver.WithRouter[router]
}

// Component B is not routed.
type b struct {
    weaver.Implements[B]
}

var _ weaver.RoutedBy[router] = (*a)(nil)
var _ weaver.Unrouted = (*b)(nil)
```

First, we implement `weaver.RoutedBy[T]` by adding an unexported `routedBy(T)`
method to `weaver.WithRouter[T]`:

```go
type WithRouter[T any] struct{}
func (WithRouter[T]) routedBy(T) {}
```

Then, we define the `weaver.RoutedBy[T]` interface.

```go
type RoutedBy[T any] interface {
    routedBy(T)
}
```

Similar to `weaver.InstanceOf[T]`, because `routedBy(T)` is unexported, only
structs that embed `weaver.WithRouter[T]` will implement the
`weaver.RoutedBy[T]` interface.

Next, we implement the `weaver.Unrouted` interface. This is a bit tricky because
we want a component that *doesn't* embed `weaver.WithRouter` to implement
`weaver.Unrouted`. How do we make a struct implement an interface based on the
absence of something? We start by defining an `implementsImpl` struct that
implements the `weaver.RoutedBy[private]` interface for an empty unexported
`private` type:

```go
type private struct{}
type implementsImpl struct{}
func (implementsImpl) routedBy(private) {}
```

Next, we embed `implementsImpl` in the `weaver.Implements[T]` struct:

```go
type Implements[T any] struct {
    implementsImpl
}
```

With this in place, any component implementation struct that embeds
`weaver.Implements[T]` will inherit the `routedBy(private)` method and therefore
implement `weaver.RoutedBy[private]`.


```go
type cache struct {
    weaver.Implements[Cache]
    ...
}

// cache inherits the routedBy(private) method from the embedded
// weaver.Implements[Cache].
var _ func(private) = cache{}.routedBy
```

We can then define `weaver.Unrouted` as an alias of `weaver.RoutedBy[private]`:

```go
type Unrouted interface {
    routedBy(private)
}
```

Now, every component struct that embeds `weaver.Implements` implements the
`weaver.Unrouted` interface. Furthermore, if a struct embeds
`weaver.WithRouter[T]`, the `routedBy(T)` method defined on
`weaver.WithRouter[T]` overrides the `routedBy(private)` method defined on the
`implementsImpl` embedded in `weaver.Implements`. This causes the struct to
implement the `weaver.RoutedBy[T]` interface instead of the `weaver.Unrouted`
interface.

```go
type cache struct {
    weaver.Implements[Cache]
    weaver.RoutedBy[router]
}

// Both weaver.Implements.implementsImpl and weaver.RoutedBy have a routedBy
// method, but cache inherits the weaver.RoutedBy method because it is less
// embedded.
var _ func(private) = cache.Implements.implementsImpl.routedBy
var _ func(router) = cache.RoutedBy.routedBy
var _ func(router) = cache.routedBy
```

Finally, `weaver generate` generates code to check, at compile time, whether
every component is routed or not:

```go
var _ weaver.RoutedBy[router] = (*calc)(nil)
```

If you add, remove, or change a `weaver.RoutedBy[T]` embedded in a component
struct, these checks will fail to build.

And finally for good measure, we can rename the `private` struct to
`if_youre_seeing_this_you_probably_forgot_to_run_weaver_generate` to make it
easier to understand why your code doesn't build:

```txt
$ go build .
# github.com/ServiceWeaver/weaver/examples/calculator
./weaver_gen.go:74:33: cannot use (*calc)(nil) (value of type *calc) as
RoutedBy[router] value in variable declaration: *calc does not implement
RoutedBy[router] (wrong type for method routedBy)
    have routedBy(if_youre_seeing_this_you_probably_forgot_to_run_weaver_generate)
    want routedBy(router)
```

## Codegen Versioning

We release a new version of the Service Weaver module and command line tool
(including `weaver generate`) [every two weeks][releases]. Every time we change
how code is generated, we assign the release a new **codegen version**. For
example, the latest codegen version as of writing this blog post is 0.17.0,
meaning that weaver module v0.17.0 has the most recent change to how we generate
code. You may update to the latest version of Service Weaver but forget to
re-run `weaver generate`.

```shell
$ go get github.com/ServiceWeaver/weaver@latest
$ go install github.com/ServiceWeaver/weaver/cmd/weaver@latest
$ go build .
# Oops! Should have run "weaver generate" first.
```

To detect this, we first introduce a new `Version` type with a [phantom type
parameter][phantoms].

```go
type Version[_ any] string
```

Next, note that we can encode version numbers as types using multidimensional
arrays. For example, version `0.17.0` is represented as `[0][17][0]struct{}`.
We define a type alias `LatestVersion` that instantiates `Version` with the
current codegen version using this encoding:

```go
type LatestVersion = Version[[CodegenMajor][CodegenMinor][CodegenPatch]struct{}]
```

`CodegenMajor`, `CodegenMinor`, and `CodegenPatch` are [constants in the weaver
module][codegen_versions] that reflect the latest codegen version:

```go
const (
    CodegenMajor = 0
    CodegenMinor = 17
    CodegenPatch = 0
)
```

Finally, `weaver generate` generates the following code:

```go
var _ LatestVersion = Version[[0][17][0]struct{}]("...")
```

Note that `weaver generate` embeds the literal values of `CodegenMajor`,
`CodegenMinor`, and `CodegenPatch` in the assignment. This assignment only
succeeds if `codegen.Version[[0][17][0]struct{}]("...")` has type
`LatestVersion`, which is true only when `0.17.0` is equal to
`CodegenMajor.CodegenMinor.CodegenPatch`.

Imagine you update the weaver module to a version where
`CodegenMajor.CodegenMinor.CodegenPatch` is `0.42.0` and you forget to re-run
`weaver generate`. The assignment above will simplify to the following:

```go
var _ Version[[0][42][0]] = Version[[0][17][0]struct{}]("...")
```

This assignment fails to build because of the mismatched versions.

Finally note that we make `Version` an alias of `string` to include an error
message that is shown when the build fails:

```go
var _ codegen.LatestVersion = codegen.Version[[0][42]struct{}](`

ERROR: You generated this file with 'weaver generate' v0.42.0 (codegen
version v0.42.0). The generated code is incompatible with the version of the
github.com/ServiceWeaver/weaver module that you're using. The weaver module
version can be found in your go.mod file or by running the following command.

    go list -m github.com/ServiceWeaver/weaver

We recommend updating the weaver module and the 'weaver generate' command by
running the following.

    go get github.com/ServiceWeaver/weaver@latest
    go install github.com/ServiceWeaver/weaver/cmd/weaver@latest

Then, re-run 'weaver generate' and re-build your code. If the problem persists,
please file an issue at https://github.com/ServiceWeaver/weaver/issues.

`)
```

## Conclusion

In this blog post, we described some of the advanced ways we use Go to detect
when you forget to run `weaver generate`. For a similar article, we recommend
reading about [how Service Weaver uses generics to implement strongly typed
metric labels][metric_labels]. If you'd like to learn more about Service Weaver,
we recommend you read [the documentation][docs].

[codegen_versions]: https://pkg.go.dev/github.com/ServiceWeaver/weaver@v0.18.0/runtime/version#pkg-constants
[components]: ..//docs.html#step-by-step-tutorial-components
[docs]: ../docs.html
[interface_checks]: https://stackoverflow.com/a/27804417
[metric_labels]: ./metric_labels.html
[phantoms]: https://wiki.haskell.org/Phantom_type
[releases]: https://github.com/ServiceWeaver/weaver/releases
[routing]: ..//docs.html#routing
[serializability]: ..//docs.html#serializable-types
