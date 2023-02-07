# Service Weaver

-   **TODO**: Link to user guide.
-   **TODO**: Link to homepage.

## What is Service Weaver?

Service Weaver is a programming framework for writing, deploying, and managing
distributed applications. You can run, test, and debug a Service Weaver
application locally on your machine, and then deploy the application to the
cloud with a single command.

```bash
$ go run .                      # Run locally.
$ weaver gke deploy weaver.toml # Run in the cloud.
```

A Service Weaver application is composed of a number **components**. A component
is represented as a regular Go interface, and components interact with each
other by calling the methods defined by these interfaces. This makes writing
Service Weaver applications easy. You don't have to write any networking or
serialization code; you just write Go. Service Weaver also provides libraries
for logging, metrics, tracing, routing, testing, and more.

You can deploy a Service Weaver application as easily as running a single
command. Under the covers, Service Weaver will dissect your binary along
component boundaries, allowing different components to run on different
machines. Service Weaver will replicate, autoscale, and co-locate these
distributed components for you. It will also manage all the networking details
on your behalf, ensuring that different components can communicate with each
other and that clients can communicate with your application.
