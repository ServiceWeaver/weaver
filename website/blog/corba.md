# CORBA vs. the Fallacies of Distributed Computing

<div class="blog-author">Michael Whittaker</div>
<div class="blog-date">April 17, 2023</div>

## Introduction

Service Weaver is a programming framework for writing distributed applications
in Go. A Service Weaver application is implemented as a set of actor-like
entities called [components][]. A component implements a set of methods, and
components interact by calling these methods. When a Service Weaver application
is deployed, some components are co-located in the same process, and others are
run in separate processes, potentially on separate machines. As a result, some
method calls are performed locally, and some are performed remotely.

This paradigm might sound familiar to you. It's similar to the distributed
object model used by CORBA, DCOM, Java RMI, and countless other frameworks that
have come and gone. It is [widely believed][why_corba_failed] that these
frameworks failed because they attempted to make remote method calls **look**
and **act** like local method calls. There are [whole papers][a_note_on_dc]
explaining that frameworks trying to unify local and remote objects are "doomed
to failure". Does this mean that Service Weaver is doomed to fail just like
CORBA?

No.

The hypothesis that CORBA failed because method calls looked like local calls
but behaved like remote calls is incorrect. CORBA actually failed for different
reasons, which we explain below. Many successful systems have made remote calls
look local (e.g., [TensorFlow][tensorflow], [Microsoft Orleans][orleans]). We
hope for Service Weaver to join this list of successful systems as it gets
adopted.

In this blog post, we explain that remote method calls in Service Weaver do
**look** like local calls, but they don't **act** like them. We'll also explain
why CORBA failed for unrelated reasons.

## Looking Local

When we say a remote method call **looks** like a local method call, we mean
they **syntactically** look the same. This is contrast to the calls' semantics,
or the way they act, which we discuss in the next section.

Typically, remote method calls are syntactically pronounced. When you issue an
RPC with [gRPC][], for example, you have to [provide protocol buffers as
arguments](https://grpc.io/docs/languages/go/basics/#simple-rpc-1), making it
clear that the arguments are being serialized and sent over the network. Some
frameworks add additional syntax to remote calls even when it is otherwise not
needed. [Ray](https://www.ray.io/), for example, requires that a remote call to
function `f(...)` be written as [`f.remote(...)`][ray_remote].

```python
# An invocation of the remote function square in Ray. Note the remote keyword.
futures = [square.remote(i) for i in range(4)]
```

However, placing a syntactic burden on developers when making remote method
calls is unnecessary. Even [*A Note on Distributed Computing*][a_note_on_dc],
the paper that argues the unification of local and remote objects is "doomed to
failure", does "not argue against pleasant programming interfaces."

Some people think that remote method calls require syntactic adornment so that
developers can tell when a remote call is happening. However, any syntactic
marker is inadequate, as remote method calls are usually performed within
functions, and the callers of these functions do not see the syntactic marker
that is "required" to tell when a remote method call is happening. For example,
is the following key-value store `Put` local or remote?

```go
// Is this key-value store Put local or remote?
resp, err := cli.Put(ctx, "sample_key", "sample_value")
```

Syntactically, it's impossible to tell. This example was taken from the [etcd
client documentation][etcd_client]. Only after digging into the implementation
of `Put` will you find that [etcd](https://etcd.io/) implements `Put` as [a gRPC
RPC under the covers][etcd_put]:

```go
var resp *pb.PutResponse
r := &pb.PutRequest{Key: op.key, Value: op.val, Lease: int64(op.leaseID), PrevKv: op.prevKV, IgnoreValue: op.ignoreValue, IgnoreLease: op.ignoreLease}
resp, err = kv.remote.Put(ctx, r, kv.callOpts...)
if err == nil {
    return OpResponse{put: (*PutResponse)(resp)}, nil
}
```

SQL queries are similar. The following query could be performed locally against
a [SQLite](https://sqlite.org/index.html) database or globally against a
[Spanner](https://cloud.google.com/spanner) database. It's not the syntax of the
program, but rather the overall context and semantics that you use to understand
how the program executes.

```sql
SELECT pid, pname
FROM parts
WHERE color == 'blue'
```

With Service Weaver, remote method calls are syntactically identical to local
method calls. They do **look** the same, and that's okay.

```go
// A remote method call in Service Weaver.
sum, err := adder.Add(ctx, 1, 2)
```

## Acting Local

When we say a remote method call **acts** like a local method call, we mean they
have the same **semantics**. This is where frameworks can get into trouble.
[The fallacies of distributed computing][fallacies] explain that remote method
calls have fundamentally different semantics than local method calls. For
example:

- Remote method calls exhibit partial (and sometimes undetectable) failures.
  Local method calls can leverage fate sharing; if something crashes, everything
  crashes.
- Remote method calls execute with significantly higher latency than local
  method calls.
- Remote method call arguments and results must be copied, but local method
  calls can use references.

Because of these fundamental semantic differences, trying to make a remote
method call act like a local method call will not end well. Take distributed
shared memory, for example. The idea behind distributed shared memory is that a
regular single-machine program can be run, without modification, across multiple
machines. The program's low-level memory loads and stores are translated to
execute against remote memory. Distributed shared memory never took off because
hiding the latency and partial failures of remote memory accesses is extremely
challenging.

Or consider NFS hard mounts, an example taken from [*A Note on Distributed
Computing*][a_note_on_dc]. If an operation on a hard mounted NFS volume fails,
the NFS client tries to mask the failure by retrying the operation indefinitely
until it succeeds. This means that if any NFS server fails, all clients
freeze until it is repaired.

While Service Weaver remote method calls *look* like local method calls, they do
not *act* like them.

- Remote methods may return an error on network and participant failures.  The
  developer is responsible for handling such errors.
- Every remote method's first argument is a `context.Context`. The
  developer is responsible for setting deadlines on these contexts to detect and
  react to method calls that are experiencing high latency.
- The developer is responsible for ensuring that all arguments and returns of a
  remote method call are serializable, which is enforced by the [Service Weaver
  code generator][generator].

Yes, these are a burden on the programmer, but that is precisely the point of
[*A Note on Distributed Computing*][a_note_on_dc]; it is a mistake to try to
isolate the programmer from the fundamental issues of remote calls. We are also
working on extending our [unit test framework][testing] to automatically inject
latency and failures to further ensure that Service Weaver applications are
resilient to the failures and delays they will encounter in a real deployment.

## Why Did CORBA Fail?

Contrary to popular belief, CORBA did not fail because of an inherent flaw with
the distributed object paradigm. [Michi Henning][michi], co-author of the book
[*Advanced CORBA Programming with C++*][corba_book], wrote an article for ACM
Queue in 2006 titled [*The Rise and Fall of CORBA*][corba_queue] that documents
why CORBA failed. Henning explains that CORBA failed because it was plagued with
technical and bureaucratic issues. For example, CORBA had unnecessarily complex
APIs, subtle language portability issues, a bloated wire protocol, unsecure and
unencrypted network traffic, no versioning support, etc. It was these issues
that sank CORBA, not the distributed object paradigm. In fact, projects like
[TensorFlow][tensorflow], [Pathways][pathways], [Ray][ray], [Microsoft
Orleans][orleans], and [Akka][akka] prove that similar paradigms can be
implemented with incredible success.

[a_note_on_dc]: https://scholar.harvard.edu/files/waldo/files/waldo-94.pdf
[akka]: https://akka.io/
[components]: ../docs.html#components
[corba_book]: https://www.informit.com/store/advanced-corba-programming-with-c-plus-plus-9780201379273
[corba_queue]: https://queue.acm.org/detail.cfm?id=1142044
[etcd_client]: https://pkg.go.dev/go.etcd.io/etcd/client/v3#section-readme
[etcd_put]: https://github.com/etcd-io/etcd/blob/217d183e5a2b2b7e826825f8218b8c4f53590a8f/client/v3/kv.go#L153-L159
[fallacies]: https://en.wikipedia.org/wiki/Fallacies_of_distributed_computing
[gRPC]: https://grpc.io/
[generator]: ../docs.html#weaver-generate
[michi]: http://www.triodia.com/
[orleans]: https://learn.microsoft.com/en-us/dotnet/orleans/overview
[pathways]: https://arxiv.org/pdf/2203.12533.pdf
[ray]: https://www.ray.io/
[ray_remote]: https://docs.ray.io/en/latest/ray-core/walkthrough.html
[tensorflow]: https://www.tensorflow.org/
[testing]: ../docs.html#testing
[why_corba_failed]: https://stackoverflow.com/a/3836026
