# Strongly Typed Metric Labels Using Generics in Go

<div class="blog-author">Michael Whittaker</div>
<div class="blog-date">August 1, 2023</div>

Service Weaver is a programming framework for writing distributed systems. An
essential part of building distributed systems is monitoring a system's behavior
with [metrics][what_are_metrics]. For a system that serves HTTP traffic, for
example, you likely want to export an `http_requests` metric that counts the
number of HTTP requests your system receives.

Metrics are typically labeled, allowing you to dissect metric measurements
across various dimensions. For example, you may want to add a `path` label to
the `http_requests` metrics that tracks the path against which HTTP requests are
issued (e.g., `"/"`, `"/users/ava"`, `"/settings/profile"`) and a `status_code`
label that tracks the HTTP response status code (e.g., `404`, `500`).

## Existing APIs

With most metric libraries, you specify metric labels as a set of key-value
pairs. For example, here is how to declare an `http_requests` counter with
`path` and `status_code` labels using [Prometheus' Go API][prometheus_api]:

```go
requests := prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "http_requests",
        Help: "Number of HTTP requests.",
    },
    []string{"path", "status_code"},
)
```

And here is how to increment the counter:

```go
requests.With(prometheus.Labels{"path": "/foo", "status_code": "404"}).Inc()
```

Similarly, here is how to declare the same metric using [OpenTelemetry's Go
API][otel_api]:

```go
requests, err := meter.Int64Counter(
    "http_requests",
    metric.WithDescription("Number of HTTP requests."),
)
```

Note that there isn't a way for us to specify the `path` and `status_code`
labels when we declare the counter. Instead, we provide the labels (what
OpenTelemetry calls attributes) when we increment the counter:

```go
requests.Add(ctx, 1, metric.WithAttributes(
    attribute.Key("path").String("/foo"),
    attribute.Key("status_code").Int(404),
))
```

## Drawbacks

Specifying labels as a set of key-value pairs like this has a number of
disadvantages.

- **Misspelling.** You can misspell label names. For example, you might
  accidentally use label name `"stats_code"` instead of `"status_code"`.
- **Mistyping.** You can use label values with the wrong type. For example, you
  might accidentally pass a status like `"404 Not Found"` instead of the status
  code `404`.
- **Misremembering.** You can misremember the name of a label completely. Was it
  `"path"` or `"endpoint"` or `"URL"`? You can accidentally use the wrong name.
- **Too many labels.** You can pass a label that wasn't defined on the metric.
  If we remove the `status_code` label, for example, we have to be careful to
  update our code everywhere that uses it.
- **Too few labels.** You can forget to pass a label. If we add a new label to a
  metric, for example, we again have to carefully update our code to use it.

## Service Weaver's Approach

Service Weaver avoids these drawbacks by integrating metric labels with Go's
type system. Specifically, Service Weaver represents metric labels as structs
and uses generics to instantiate metrics. Here's how to declare the
`http_requests` metric using [Service Weaver's API][weaver_api]:

```go
type labels struct {
    Path       string `weaver:"path"`
    StatusCode int    `weaver:"status_code"`
}

var requests = metrics.NewCounterMap[labels](
    "http_requests",
    "Number of HTTP requests.",
)
```

And here's how to increment the counter:

```go
requests.Get(labels{Path: "/foo", StatusCode: 404}).Inc()
```

Labels can be any struct where every field is a string, bool, or integer.
`NewCounterMap` panics if you call it with an invalid label struct. By
leveraging Go's type system, Service Weaver's API avoids the disadvantages of
key-value labels described above.

- **Misspelling.** If you misspell a field name, your code will not compile
  (e.g., `labels{Path: "/foo", StatsCode: 404}`).
- **Mistyping.** If you pass a label value of the wrong type, your code will not
  compile (e.g., `labels{Path: "/foo", StatusCode: "404 Not Found"}`).
- **Misremembering.** If you use the wrong field name, your code will not
  compile (e.g., `labels{Endpoint: "/foo", StatusCode: 404}`).
- **Too many labels.** If you pass a label that does not exist, your code will
  not compile (e.g., `labels{Path: "/foo", StatusCode: 404, ContentLength: 42}`).
- **Too few labels.** This one is a bit tricky because Go does not require you
  to initialize every field in a struct. However, if you omit field names in a
  struct literal, you must provide every field or your code will not compile
  (e.g., `labels{"/foo"}`).

These strongly typed metric labels are just one of the ways that Service Weaver
makes it easier to write distributed systems. Read [the Service Weaver
documentation][docs] to learn about more about [metrics][metrics] (including
counters, gauges, and histograms) as well as other useful features.

[docs]: ../docs.html
[metrics]: ../docs.html#metrics
[otel_api]: https://pkg.go.dev/go.opentelemetry.io/otel/metric
[prometheus_api]: https://pkg.go.dev/github.com/prometheus/client_golang/prometheus
[weaver_api]: https://pkg.go.dev/github.com/ServiceWeaver/weaver/metrics
[what_are_metrics]: https://prometheus.io/docs/introduction/overview/#what-are-metrics
