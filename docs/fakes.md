# Fakes

When testing a Service Weaver application, with either weavertest or the
simulator, you can replace the real implementation of a component with a fake
implementation. See [`clock_test.go`][clock_test] for example. However, it is
sometimes necessary to fake at a granularity finer than that of a component;
i.e. to fake something within a component rather than fake a component in its
entirety.

## Motivating Example

Consider the following banking application as a motivating example. The bank is
backed by a relational database that records every user's bank account balance.
We begin by defining a `DB` interface to interact with the database, and a
`postgres` implementation of the interface:

```go
type DB interface {
    Get(ctx context.Context, user string) (int, error)
    Set(ctx context.Context, user string, balance int) error
}

type postgres struct { ... }
func (p *postgres) Get(ctx context.Context, user string) (int, error) { ... }
func (p *postgres) Set(ctx context.Context, user string, balance int) error { ... }
```

Next, we define a `Bank` component:

```go
type Bank interface {
    Transfer(ctx context.Context, from, to string, amount int) error
}

type bank struct {
    weaver.Implements[Bank]
    db DB
}

func (b *bank) Init(context.Context) error {
    b.db = &postgres{...}
    return nil
}

func (b *bank) Transfer(ctx context.Context, from, to string, amount int) error {
    fromBalance, _ := b.db.Get(ctx, from)
    toBalance, _ := b.db.Get(ctx, to)
    b.db.Set(ctx, from, fromBalance - amount)
    b.db.Set(ctx, to, toBalance + amount)
    return nil
}
```

Testing the `Bank` component with a real Postgres database is both slow and
cumbersome.

- **Slow.** On my machine, inserting a single row into a Postgres table running
  on a tmpfs file system with fsync disabled has a peak throughput of roughly
  8,000 inserts per second.
- **Cumbersome.** Running a Postgres database for a test or simulation is
  non-trivial. Libraries like [dockertest][] and [testcontainers][] help
  automate the process, but they require Docker. Running a Postgres database
  without Docker is challenging.

Instead, it is faster and more convenient to test the `Bank` component with a
fake implementation of the database:

```go
type fakeDB struct {
    mu sync.Mutex
    balances map[string]int
}

var _ DB = &fakeDB{}

func (f *fakeDB) Get(ctx context.Context, user string) (int, error) {
    f.mu.Lock()
    defer f.mu.Unlock()
    return f.balances[user], nil
}

func (f *fakeDB) Set(ctx context.Context, user string, balance int) error {
    f.mu.Lock()
    defer f.mu.Unlock()
    f.balances[user] = balance
    return nil
}
```

In the rest of this document, we discuss various ways to fake stuff within a
component. Some options are discussed in the context of the simulator, but they
apply to weavertest as well.

## Option 1: Global Variables

We can inject fakes into a component using global variables.

```go
// Global DB. Can be set by tests.
var db DB

func (b *bank) Init(ctx context.Context) error {
    // Use the global DB, if not nil.
    if db != nil {
        b.db = db
    } else {
        b.db = &postgres{...}
    }
    return nil
}
```

This approach is bad for all the reasons using global variables is bad. For
simulation, it is even worse because the simulator runs many instances of the
`Bank` component in parallel, and each instance needs its own copy of a fake
database.

## Option 2: Context Hacking

We can inject fakes into the context passed to a component's `Init` method. This
requires an additional mechanism to register a context to pass to the
components. For the simulator, we can add a `RegisterContext` method to the
`Registrar`.

```go
func (b *bank) Init(ctx context.Context) error {
    if db := ctx.Get("db"); db != nil {
        b.db = db.(DB)
    } else {
        b.db = &postgres{...}
    }
    return nil
}

// In a *_test.go file:
type BankWorkload struct { ... }

func (b *BankWorkload) Init(r sim.Registrar) error {
    // Inside a workload's Init method, register a context that will be passed
    // to the Init method of every component.
    ctx := context.Background()
    r.RegisterContext(context.WithValue(ctx, "db", &fakeDB{}))
    return nil
}

...
```

## Option 3: Component Refactoring

Rather than introducing a new mechanism to fake things within a component, we
can encourage developers to refactor their code into smaller components that can
then be faked in their entirety. For our bank example, this approach works quite
well. We separate `DB` into its own component and then fake it.

I think this is the best approach when it works out nicely, but it unfortunately
doesn't always work out nicely. The code we're faking may not have an API that
can wrapped up into a component. The API being faked may involve
non-serializable types, for example.

This approach can also lead to co-location complexities. Imagine we make `DB` a
component. For performance reasons, we want every component that uses `DB` to be
co-located with `DB`. However, we may not want every component that uses `DB` to
be co-located with each other. This is not possible today, though we can make it
possible by complicating how we co-locate components.

## Option 4: Injection Functions

We can allow a developer to register an "injection function" for a component.
The injection function is called after the component is constructed and can
replace certain fields with fakes. Here's how that might look for the simulator:

```go
func (b *bank) Init(ctx context.Context) error {
    b.db = &postgres{...}
    return nil
}

// In a *_test.go file:
type BankWorkload struct { ... }

func (b *BankWorkload) Init(r sim.Registrar) error {
    // Inside a workload's Init method, register an injection function that will
    // be called on every replica of the bank component.
    fake := &fakeDB{}
    r.RegisterInjection(func(b *bank) { b.db = fake })
    return nil
}

...
```

This approach has the awkwardness that a component is fully constructed and then
afterwards has some of its fields replaced. In the example above, we would need
to add some extra logic to the `bank`'s `Init` method to not connect to a
Postgres database when being tested.

## Option 5: Constructor Functions

We can allow a developer to register a constructor function for a component.
The constructor function is called to create a new instance of the component.
Here's how that might look for the simulator:

```go
func (b *bank) Init(ctx context.Context) error {
    if b.db != nil {
        b.db = &postgres{...}
    }
    return nil
}

func NewBankWithDB(db DB) (*bank, error) {
    return &bank{db: db}, nil
}

// In a *_test.go file:
type BankWorkload struct { ... }

func (b *BankWorkload) Init(r sim.Registrar) error {
    // Inside a workload's Init method, register a constructor function that
    // will be called to create every replica of the bank component.
    fake := &fakeDB{}
    r.RegisterConstructor(func() (*bank, error) {
        return NewBankWithDB(fake)
    })
    return nil
}

...
```

Constructor functions are similar to injector functions, but avoid some issues
around redundantly initializing fields. Constructor functions are awkward,
though, because component structs contain fields like `weaver.Implements`,
`weaver.Ref`, `weaver.Config` that a user has no way of initializing.

[clock_test]: ../examples/fakes/clock_test.go
[dockertest]: https://github.com/ory/dockertest
[testcontainers]: https://testcontainers.com
