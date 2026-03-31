# seedling Guide

Practical workflows and API patterns for using seedling in your tests. Start with [README Installation](../README.md#-installation) and [Quick Start](../README.md#-quick-start) if you haven't set up seedling yet.

## Core Workflows

### Basic inserts

Use `InsertOne` when a test needs a single root record and all required parents.

```go
result := seedling.InsertOne[Task](t, db)
task := result.Root()
```

Use `InsertMany` when a test needs multiple records of the same type.

```go
users := seedling.InsertMany[User](t, db, 3,
    seedling.Seq("Name", func(i int) string {
        return fmt.Sprintf("user-%d", i)
    }),
)
```

Use `InsertManyE` when you need the full batch result for debugging or cleanup.

```go
result, err := seedling.InsertManyE[User](ctx, db, 3,
    seedling.Seq("Name", func(i int) string {
        return fmt.Sprintf("user-%d", i)
    }),
)
if err != nil {
    _ = result.CleanupE(ctx, db)
    t.Fatal(err)
}

users := result.Roots()
_ = users

company, ok := result.NodeAt(1, "company")
_ = company
_ = ok
```

`InsertMany` batch-shares auto-created `BelongsTo` parents when each record resolves to the same static relation options.

```go
tasks := seedling.InsertMany[Task](t, db, 2,
    seedling.Ref("project", seedling.Set("Name", "shared-project")),
)
```

In this example, `project` is inserted once and both tasks point to the same row.

- Sharing applies only to auto-created `BelongsTo` relations in `InsertMany`
- The dedupe key is the relation path plus the resolved option tree for that path, after `Seq` and `SeqRef` are expanded per record
- Static option trees made of `Set`, nested `Ref`, and `Omit` can be shared
- `Use`, `With`, `Generate`, `When`, and rand-driven options make that relation non-shareable
- If the resolved options differ per record, seedling inserts separate parent rows

### Plan-first workflows

Use `Build` when you want to inspect or validate the graph before executing inserts.

```go
plan := seedling.Build[Task](t,
    seedling.Ref("project", seedling.Set("Name", "renewal")),
)

if err := plan.Validate(); err != nil {
    t.Fatal(err)
}

result := plan.Insert(t, db)
task := result.Root()
_ = task
```

When a plan includes `AfterInsert` / `AfterInsertE`, remember that those callbacks are captured once at `Build` time. Reusing the same `Plan` also reuses any closure state captured by those callbacks, so rebuild the plan if each execution needs isolated callback state.

### Reusing existing rows

Use `Use` to bind a relation to a row that already exists.

```go
company := seedling.InsertOne[Company](t, db).Root()
user := seedling.InsertOne[User](t, db,
    seedling.Use("company", company),
).Root()
_ = user
```

### Selective insertion with Only

Use `Only` when a blueprint has many relations but the test only needs a subset. The planner builds only the necessary subgraph, skipping relations not listed in `Only`. `DebugString` / `DryRunString` reflect this lazily built graph.

```go
// Insert task + project subtree only. Assignee and its dependencies are skipped.
result := seedling.InsertOne[Task](t, db,
    seedling.Only("project"),
)

// Only() with no arguments inserts just the root record.
result := seedling.InsertOne[Task](t, db, seedling.Only())
```

`Only` also works with `InsertMany`. The filter is applied per root before batch sharing is resolved, so matching `BelongsTo` parents can still be shared across the batch.

### Transaction auto-rollback

Use `WithTx` to wrap each test in a transaction that rolls back on cleanup. This is the simplest way to isolate test data without manual deletion.

```go
func TestUser(t *testing.T) {
    tx := seedling.WithTx(t, db)
    user := seedling.InsertOne[User](t, tx).Root()
    // tx.Rollback() is called automatically when the test finishes.
    _ = user
}
```

For more control (custom `sql.TxOptions`, registry binding), use `NewTestSession` instead.

If you use pgx directly, `github.com/mhiro2/seedling/seedlingpgx` provides the same workflow for `pgxpool.Pool` or `*pgx.Conn`.

```go
func TestUser(t *testing.T) {
    tx := seedlingpgx.WithTx(t, pool)
    user := seedling.InsertOne[User](t, tx).Root()
    _ = user
}
```

## Common Options

- `Set`: override one field by Go struct field name
- `Ref`: customize the auto-created node behind a relation and explicitly enable an optional relation
- `Use`: reuse an existing record instead of inserting a new relation target
- `Omit`: skip an optional relation
- `Only`: restrict insertion to specific relation subtrees
- `When`: expand a relation only when the current record matches a condition, including optional relations when the predicate returns true
- `With`: mutate the root struct with full type safety
- `Generate` + `WithSeed` / `WithRand`: generate deterministic values for property-style tests
- `WithInsertLog`: observe insert steps and FK assignments during execution

For runnable examples of these options, see [`example_test.go`](../example_test.go).

## Relationship Patterns

- `BelongsTo`: insert required parent rows and write the parent key into the child
- `HasMany`: insert children automatically from a parent blueprint using `Count`
- `ManyToMany`: create related rows and join-table rows together
- Composite keys: use `PKFields`, `LocalFields`, and `RemoteFields`

The execution model and graph expansion rules are documented in [ARCHITECTURE.md](../ARCHITECTURE.md).

## SQL Integration

seedling does not generate SQL at runtime. Your blueprint owns the `Insert` and optional `Delete` callbacks, so the library works with any DB abstraction that your code already uses.

- sqlc: map `Insert` callbacks to generated query methods. Prefer `seedling-gen sqlc --config ...` for automatic setup
- `database/sql`: pass `*sql.DB` or `*sql.Tx`
- pgx: pass your pool or transaction handle, or use `github.com/mhiro2/seedling/seedlingpgx` for rollback-on-cleanup helpers
- GORM: use `-gorm` to generate blueprints with `gorm.DB`-based Insert/Delete callbacks
- ent: use `-ent` to generate blueprints with ent fluent builder Insert/Delete callbacks
- Atlas HCL: use `-atlas` to generate blueprints from Atlas schema definitions

When you use `database/sql`, [`WithTx`](https://pkg.go.dev/github.com/mhiro2/seedling#WithTx) is the easiest way to get a rollback-on-cleanup transaction. [`NewTestSession`](https://pkg.go.dev/github.com/mhiro2/seedling#NewTestSession) offers the same with registry binding and custom `sql.TxOptions`. For pgx, use [`seedlingpgx.WithTx`](https://pkg.go.dev/github.com/mhiro2/seedling/seedlingpgx#WithTx) or [`seedlingpgx.NewTestSession`](https://pkg.go.dev/github.com/mhiro2/seedling/seedlingpgx#NewTestSession).

## Debugging And Cleanup

- `Plan.DebugString`: inspect the dependency tree before inserts
- `Plan.DryRunString`: inspect insert order and FK assignments without executing inserts
- `Result.DebugString`: inspect inserted nodes with primary-key values
- `Result.Node(name)`: returns the lexicographically smallest matching node ID when multiple nodes share the same blueprint name
- `Result.Nodes(name)`: returns all matching nodes in node ID order
- `Result.Cleanup` / `CleanupE`: delete inserted rows in reverse dependency order when transaction rollback is not available
- `BatchResult.DebugString`: inspect the full batch execution graph with primary-key values
- `BatchResult.Node(name)`: searches across the full batch, so use it only when cross-root ambiguity is acceptable
- `BatchResult.NodeAt(rootIndex, name)` / `NodesForRoot(rootIndex, name)`: inspect one root and its shared ancestors without mixing in sibling roots
- `BatchResult.Cleanup` / `CleanupE`: delete rows inserted by `InsertManyE`; cleanup is fail-fast and stops at the first delete error

## CLI

[`seedling-gen`](../cmd/seedling-gen) generates model and blueprint skeletons from multiple input sources.

Install the CLI:

```bash
# Homebrew (macOS / Linux)
brew install --cask mhiro2/tap/seedling-gen
```

```bash
# Go toolchain
go install github.com/mhiro2/seedling/cmd/seedling-gen@latest
```

Examples:

```bash
# SQL DDL
seedling-gen sql --pkg blueprints schema.sql

# sqlc config: auto-resolves schema, output dir, and import path from sqlc.yaml
seedling-gen sqlc --config sqlc.yaml --pkg blueprints

# sqlc manual mode: use generated Go files plus an explicit schema.sql
seedling-gen sqlc --dir ./internal/db --import-path github.com/you/app/internal/db --pkg blueprints schema.sql

# GORM models: parses Go source with gorm struct tags
seedling-gen gorm --dir ./models --import-path github.com/you/app/models --pkg blueprints

# ent schemas: parses ent schema directory (Fields/Edges methods)
seedling-gen ent --dir ./ent/schema --import-path github.com/you/app/ent --pkg blueprints

# Atlas HCL: parses Atlas schema file
seedling-gen atlas --pkg blueprints schema.hcl
```

All subcommands support `--pkg` (generated package name) and `--out` (output file path). The `sql` and `sqlc` subcommands also support `--dialect` (`auto`, `postgres`, `mysql`, `sqlite`) as a validation hint. The SQL parser itself uses the same logic for all dialects, so `--dialect` does not change parsing behavior.

All subcommands also support diagnostic output modes:

- `--explain`: print the parsed schema/model metadata plus the inferred blueprint relations instead of generated Go code
- `--json`: print the same diagnostic report as JSON, which is useful for tooling or CI checks

The `sqlc` subcommand has two input modes:

- `--config`: read `sqlc.yaml` and auto-resolve schema files, output directory, and Go import path
- `--dir` + `--import-path` + `<schema.sql>`: manually point at generated sqlc Go files and the schema DDL

When `--out` is specified, the output is written atomically via a temporary file so that a generation failure never leaves a partial file on disk.

## Faker Locales

The `faker` package supports multiple locales for generating locale-appropriate fake data. Use `NewWithLocale` to select a locale:

```go
seedling.Generate(func(r *rand.Rand, u *User) {
    f := faker.NewWithLocale(r, "ja")
    u.Name  = f.Name()   // "佐藤太郎"
    u.Email = f.Email()   // "taro.sato@example.com"
    u.Phone = f.Phone()   // "+81-03-1234-5678"
})
```

Supported locales: `en` (default), `ja`, `zh`, `ko`, `de`, `fr`.

`New(r)` defaults to `"en"` and is fully backward compatible.

## Examples

- [basic](../examples/basic) -- register blueprints and insert rows with automatic parent creation
- [quickstart](../examples/quickstart) -- generated-style `RegisterBlueprints()` flow that matches the README Quick Start
- [custom-defaults](../examples/custom-defaults) -- customize values with `Set`, `With`, and `Generate`
- [reuse-parent](../examples/reuse-parent) -- reuse existing rows with `Use`
- [batch-insert](../examples/batch-insert) -- batch inserts with shared `Ref` dependencies and per-row `SeqRef` overrides
- [with-tx](../examples/with-tx) -- use `seedling.WithTx` for automatic rollback with `database/sql`
- [sqlc](../examples/sqlc) -- wire blueprints to sqlc-generated query code

## More References

- [Architecture](../ARCHITECTURE.md) -- internal pipeline design (planner, graph, executor)
- [README](../README.md) -- project overview, Quick Start, and comparison table
- [pkg.go.dev package docs](https://pkg.go.dev/github.com/mhiro2/seedling)
- [faker package docs](https://pkg.go.dev/github.com/mhiro2/seedling/faker)
