# Wrap

This directory contains a Service Weaver web application that wraps text to 80
characters. The application has a main component and a `Wrapper` component.

## How to run?

To run this application locally in a single process, run `go run .` To run the
application locally across multiple processes, use `weaver multi deploy`.

```console
$ go run .                        # Run in a single process.
$ weaver multi deploy weaver.toml # Run in multiple processes.
```

## How to interact with the application?

If running locally, open `localhost:9000` in a browser.

TODO(mwhittaker): Run on GKE.
