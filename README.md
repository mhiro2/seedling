<p align="center">
  <img src="assets/logo.png" width="256" height="256" alt="seedling logo">
</p>

<h1 align="center">seedling</h1>

<p align="center">
  <strong>Dependency-aware test data builder for Go and SQL databases.</strong><br>
  seedling lets tests create only the rows they need while automatically resolving foreign-key dependencies in the correct order. You provide the insert logic. seedling handles planning, FK assignment, and execution order.
</p>

<p align="center">
  <a href="https://pkg.go.dev/github.com/mhiro2/seedling">
    <img src="https://pkg.go.dev/badge/github.com/mhiro2/seedling.svg" alt="Go Reference">
  </a>
  <a href="https://deepwiki.com/mhiro2/seedling"><img src="https://img.shields.io/badge/DeepWiki-mhiro2%2Fseedling-blue.svg?logo=data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAACwAAAAyCAYAAAAnWDnqAAAAAXNSR0IArs4c6QAAA05JREFUaEPtmUtyEzEQhtWTQyQLHNak2AB7ZnyXZMEjXMGeK/AIi+QuHrMnbChYY7MIh8g01fJoopFb0uhhEqqcbWTp06/uv1saEDv4O3n3dV60RfP947Mm9/SQc0ICFQgzfc4CYZoTPAswgSJCCUJUnAAoRHOAUOcATwbmVLWdGoH//PB8mnKqScAhsD0kYP3j/Yt5LPQe2KvcXmGvRHcDnpxfL2zOYJ1mFwrryWTz0advv1Ut4CJgf5uhDuDj5eUcAUoahrdY/56ebRWeraTjMt/00Sh3UDtjgHtQNHwcRGOC98BJEAEymycmYcWwOprTgcB6VZ5JK5TAJ+fXGLBm3FDAmn6oPPjR4rKCAoJCal2eAiQp2x0vxTPB3ALO2CRkwmDy5WohzBDwSEFKRwPbknEggCPB/imwrycgxX2NzoMCHhPkDwqYMr9tRcP5qNrMZHkVnOjRMWwLCcr8ohBVb1OMjxLwGCvjTikrsBOiA6fNyCrm8V1rP93iVPpwaE+gO0SsWmPiXB+jikdf6SizrT5qKasx5j8ABbHpFTx+vFXp9EnYQmLx02h1QTTrl6eDqxLnGjporxl3NL3agEvXdT0WmEost648sQOYAeJS9Q7bfUVoMGnjo4AZdUMQku50McDcMWcBPvr0SzbTAFDfvJqwLzgxwATnCgnp4wDl6Aa+Ax283gghmj+vj7feE2KBBRMW3FzOpLOADl0Isb5587h/U4gGvkt5v60Z1VLG8BhYjbzRwyQZemwAd6cCR5/XFWLYZRIMpX39AR0tjaGGiGzLVyhse5C9RKC6ai42ppWPKiBagOvaYk8lO7DajerabOZP46Lby5wKjw1HCRx7p9sVMOWGzb/vA1hwiWc6jm3MvQDTogQkiqIhJV0nBQBTU+3okKCFDy9WwferkHjtxib7t3xIUQtHxnIwtx4mpg26/HfwVNVDb4oI9RHmx5WGelRVlrtiw43zboCLaxv46AZeB3IlTkwouebTr1y2NjSpHz68WNFjHvupy3q8TFn3Hos2IAk4Ju5dCo8B3wP7VPr/FGaKiG+T+v+TQqIrOqMTL1VdWV1DdmcbO8KXBz6esmYWYKPwDL5b5FA1a0hwapHiom0r/cKaoqr+27/XcrS5UwSMbQAAAABJRU5ErkJggg==" alt="DeepWiki"></a>
  <a href="../../releases"><img alt="Release" src="https://img.shields.io/github/v/release/mhiro2/seedling"></a>
  <a href="https://github.com/mhiro2/seedling/actions/workflows/ci.yaml">
    <img src="https://github.com/mhiro2/seedling/actions/workflows/ci.yaml/badge.svg?branch=main" alt="CI">
  </a>
  <img src="https://img.shields.io/badge/license-MIT-green?style=flat-square" alt="MIT License">
</p>

---

## ✨ Why seedling?

Manually wiring FK dependencies across 4 tables:

```go
func TestCreateTask(t *testing.T) {
    company, err := db.InsertCompany(ctx, InsertCompanyParams{Name: "acme"})
    if err != nil { t.Fatal(err) }

    user, err := db.InsertUser(ctx, InsertUserParams{
        Name: "alice", CompanyID: company.ID,
    })
    if err != nil { t.Fatal(err) }

    project, err := db.InsertProject(ctx, InsertProjectParams{
        Name: "renewal", CompanyID: company.ID,
    })
    if err != nil { t.Fatal(err) }

    task, err := db.InsertTask(ctx, InsertTaskParams{
        Title: "design", ProjectID: project.ID, AssigneeUserID: user.ID,
    })
    if err != nil { t.Fatal(err) }

    _ = task
}
```

With seedling, the graph is resolved automatically:

```go
func TestCreateTask(t *testing.T) {
    result := seedling.InsertOne[Task](t, db)
    task := result.Root()
    _ = task
}
```

seedling handles FK ordering, graph expansion, and cleanup so your tests stay focused on what matters:

- 🪶 Zero runtime dependencies in the core module; optional DB helpers live in companion packages
- 🔗 Automatic FK resolution with topological insert ordering and minimal graph expansion
- 🌿 First-class relation kinds: `BelongsTo`, `HasMany`, `ManyToMany`, plus composite keys
- 🔧 Per-test overrides via `Set`, `Use`, and `Ref`
- ♻️ `WithTx` for auto-rollback transactions -- no manual cleanup
- 🔌 Works with sqlc, `database/sql`, pgx, GORM, ent, or any other DB handle you own
- 🎲 Deterministic fake data via [`seedling/faker`](https://pkg.go.dev/github.com/mhiro2/seedling/faker) with multi-locale support (en, ja, zh, ko, de, fr)

For advanced features such as `InsertMany`, batch sharing, `Only`, `When`, and dry runs, see the [Guide](./docs/guide.md).

## 📦 Installation

Add an import in your code, then let the toolchain record the dependency:

```go
import "github.com/mhiro2/seedling"
```

Use the same pattern for companion packages when you need them, for example [`seedling/faker`](https://pkg.go.dev/github.com/mhiro2/seedling/faker) (`github.com/mhiro2/seedling/faker`) or [`seedlingpgx`](https://pkg.go.dev/github.com/mhiro2/seedling/seedlingpgx) (`github.com/mhiro2/seedling/seedlingpgx`).

Install the `seedling-gen` CLI (pick one):

```bash
# Homebrew (macOS / Linux) — [third-party tap](https://github.com/mhiro2/homebrew-tap)
brew install --cask mhiro2/tap/seedling-gen
```

```bash
# Go toolchain
go install github.com/mhiro2/seedling/cmd/seedling-gen@latest
```

## 🚀 Quick Start

The shortest path is two steps: generate blueprints from your schema, then call `InsertOne` in a test.

1. **Generate blueprints from your schema**

   ```bash
   # From SQL DDL
   seedling-gen sql --pkg testutil --out blueprints.go schema.sql
   ```

   Other input sources are supported (sqlc, GORM, ent, Atlas) -- see the [Guide](./docs/guide.md#cli) for the full list.

   This generates struct types, `NewRegistry()`, `RegisterBlueprints(reg)`, deterministic `Defaults` for common scalar fields, relations, and Insert stubs. Fill in the `// TODO` callbacks with your DB logic:

   ```go
   Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
       return insertCompany(ctx, db, v) // your DB call
   },
   ```

   Generated `Defaults` intentionally skip primary keys, relation FK fields, and unsupported custom types. They are meant to make the first insert usable with zero setup, not to satisfy every unique or business constraint automatically.

   The snippets below assume the generated package is named `testutil`.
   For a runnable minimal version of this flow, see [examples/quickstart](./examples/quickstart).

2. **Use it in tests**

   ```go
   func TestUser(t *testing.T) {
       reg := testutil.NewRegistry()

       result := seedling.NewSession[testutil.User](reg).InsertOne(t, db)
       user := result.Root()

       if user.ID == 0 {
           t.Fatal("expected user ID to be set")
       }
       if user.CompanyID == 0 {
           t.Fatal("expected company to be inserted automatically")
       }
   }
   ```

That is the entire happy path. The next section shows the handful of patterns you reach for once tests grow.

## 🧩 Typical Use Cases

These three patterns cover the majority of real test code. Anything beyond them lives in the [Guide](./docs/guide.md).

> [!NOTE]
> All snippets below assume the generated package (e.g. from `seedling-gen sql --pkg testutil schema.sql`) is imported as `testutil`, and `reg := testutil.NewRegistry()` has been called. The package name is set by `--pkg`; rename to whatever fits your project.

### Override a field

Use `Set` when a test needs a specific column value:

```go
result := seedling.NewSession[testutil.User](reg).InsertOne(t, db,
    seedling.Set("Name", "alice"),
)
user := result.Root()
_ = user
```

### Reuse an existing parent row

Use `Use` to bind a relation to a row you already inserted, instead of letting seedling create another one:

```go
company := seedling.NewSession[testutil.Company](reg).InsertOne(t, db).Root()

result := seedling.NewSession[testutil.User](reg).InsertOne(t, db,
    seedling.Use("company", company),
)
user := result.Root()
_ = user
```

### Auto-rollback per test

Wrap the test in a transaction that rolls back automatically on cleanup. No manual deletion, no leaking state across tests.

```go
func TestUser(t *testing.T) {
    tx := seedling.WithTx(t, db) // rollback runs at t.Cleanup
    user := seedling.NewSession[testutil.User](reg).InsertOne(t, tx).Root()
    _ = user
}
```

For a runnable example, see [examples/with-tx](./examples/with-tx). For pgx-based projects, [`seedlingpgx.WithTx`](https://pkg.go.dev/github.com/mhiro2/seedling/seedlingpgx#WithTx) provides the same workflow.

## 🩺 Diagnostics

When a graph misbehaves, you usually want to answer one of three questions: _what would be inserted_, _what was inserted_, or _how do I clean it up_. seedling exposes one helper for each.

### Inspect the plan before executing

Use `Build` to construct the plan without inserting, then print the dependency tree or the dry-run insert order:

```go
plan := seedling.NewSession[testutil.Task](reg).Build(t,
    seedling.Ref("project", seedling.Set("Name", "renewal")),
)

t.Log(plan.DebugString())   // dependency tree
t.Log(plan.DryRunString())  // insert order + FK assignments

result := plan.Insert(t, db)
_ = result
```

### Trace inserts at runtime

`WithInsertLog` reports each insert step (table, primary key, FK assignments) as it happens:

```go
result := seedling.NewSession[testutil.Task](reg).InsertOne(t, db,
    seedling.WithInsertLog(func(log seedling.InsertLog) {
        t.Logf("step %d: %s (fks: %v)", log.Step, log.Table, log.FKBindings)
    }),
)
_ = result
```

### Clean up when transactions are not an option

If you cannot use `WithTx`, call `Result.Cleanup` to delete inserted rows in reverse dependency order:

```go
result := seedling.NewSession[testutil.User](reg).InsertOne(t, db)
t.Cleanup(func() { result.Cleanup(t, db) })
```

The [Guide](./docs/guide.md#debugging-and-cleanup) lists the full set of debugging APIs, including `BatchResult.NodeAt` for `InsertMany` graphs.

## ⚖️ Comparison

| Tool | Main model | Strong at | Not designed for |
| --- | --- | --- | --- |
| seedling | Dependency-aware builders with DB callbacks | Per-test graph generation, automatic FK resolution, type-safe overrides, graph inspection, codegen | Bulk loading large static fixture files |
| [eyo-chen/gofacto](https://github.com/eyo-chen/gofacto) | Generic factory with explicit FK associations | Ergonomic zero-config field filling, `WithOne`/`WithMany` associations, multi-DB support | Automatic graph resolution, minimal graph expansion |
| [go-testfixtures/testfixtures](https://github.com/go-testfixtures/testfixtures) | Fixture files loaded into DB | Stable predefined datasets for integration tests | Relation-aware per-test graph construction |
| [bluele/factory-go](https://github.com/bluele/factory-go) | In-memory object factories | Flexible object construction and traits-like composition | Planning SQL insert order across FK graphs |
| [brianvoe/gofakeit](https://github.com/brianvoe/gofakeit) | Fake data generator | Realistic random values | Database insertion orchestration or relation expansion |

## 📂 Examples

- [basic](./examples/basic): register blueprints and insert rows with automatic parent creation
- [quickstart](./examples/quickstart): generated-style `NewRegistry()` / `RegisterBlueprints(reg)` flow that matches the README Quick Start
- [custom-defaults](./examples/custom-defaults): customize values with `Set`, `With`, and `Generate`
- [reuse-parent](./examples/reuse-parent): reuse existing rows with `Use`
- [batch-insert](./examples/batch-insert): batch inserts with shared `Ref` dependencies and per-row `SeqRef` overrides
- [with-tx](./examples/with-tx): `database/sql` transaction helper with `seedling.WithTx`
- [sqlc](./examples/sqlc): wire blueprints to sqlc-generated query code
- pgx transactions: use `github.com/mhiro2/seedling/seedlingpgx` with `pgxpool.Pool` or `*pgx.Conn`
- GORM / ent / Atlas: use the `seedling-gen gorm`, `seedling-gen ent`, or `seedling-gen atlas` subcommands to generate blueprints from your existing schema definitions

## 📚 Learn More

- [Guide](./docs/guide.md) -- workflows, full option reference (`Only`, `When`, `InsertMany`, batch sharing, ...), and integration patterns
- [Architecture](./ARCHITECTURE.md) -- internal pipeline design (planner, graph, executor)
- [Agent Skill: seedling-gen CLI](./skills/seedling-gen-cli/SKILL.md) -- instructions for AI agents that need to choose the right generator mode and scaffold blueprints
- [Agent Skill: seedling test setup](./skills/seedling-test-setup/SKILL.md) -- instructions for AI agents that write Go tests using seedling blueprints
- [pkg.go.dev API reference](https://pkg.go.dev/github.com/mhiro2/seedling) -- full type and function docs

## 📝 License

MIT
