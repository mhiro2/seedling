# seedling Guide

Practical workflows and API patterns for using seedling in your tests. Start with [README Installation](../README.md#-installation) and [Quick Start](../README.md#-quick-start) if you haven't set up seedling yet.

## Core Workflows

The snippets in this guide call the package-level helpers (`seedling.InsertOne`, `seedling.InsertMany`, `seedling.Build`, ...), which read the global registry, to keep the examples short. In real tests prefer a test-local registry: create one with the generated `NewRegistry()` (or `seedling.NewRegistry()` + `MustRegisterTo`) and call the same methods on `seedling.NewSession[T](reg)`, as the README Quick Start does. A per-test registry keeps blueprint state isolated between tests and packages.

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

`InsertManyE` and `Plan.InsertE` return successful nodes together with an execution error when insertion stops partway through. The partial result's cleanup methods delete only those successful nodes. A partial `BatchResult` counts and lists only completed roots through `Len` and `Roots`; `RootAt` and `NodeAt` keep the original request indices and report a missing result for roots that did not complete.

When transaction rollback is unavailable, use `InsertOneE`, `Plan.InsertE`, or `InsertManyE` so an error path can call cleanup on the returned partial result. The `testing.TB` convenience wrappers (`InsertOne`, `Plan.Insert`, and `InsertMany`) still fail the test immediately and therefore do not expose partial results.

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

`Only` is an explicit minimal-graph escape hatch: relations not listed are skipped even when they are `Required`, leaving their FK fields at the zero value. Make sure the excluded relations are nullable, or list them in `Only`, when the schema enforces `NOT NULL` foreign keys. Combining `Only` with an explicit `Use` / `Ref` / `When` on a relation outside the set is rejected as a contradiction rather than silently dropped.

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

- `Set`: override one field by Go struct field name. Sets apply in the order written, so a later `Set` wins, including one that sets a field promoted from an embedded struct an earlier `Set` replaced
- `Ref`: customize the auto-created node behind a relation and explicitly enable an optional relation
- `Use`: reuse an existing record instead of inserting a new relation target
- `Omit`: skip an optional relation
- `Only`: restrict insertion to specific relation subtrees
- `When`: expand a relation only when the current record matches a condition, including optional relations when the predicate returns true
- `With`: mutate the root struct with full type safety
- `Generate` + `WithSeed` / `WithRand`: generate deterministic values for property-style tests
- `WithInsertLog`: observe insert steps and FK assignments during execution

For runnable examples of these options, see [`example_test.go`](../example_test.go).

Options resolve the same way on every insert path (`InsertOne`, `InsertMany`, `Build`, and the `Builder`). Traits expand in place first, and inside `InsertMany` the `Seq`, `SeqRef`, and `SeqUse` options are resolved for each record during that expansion, so they also work when defined in a blueprint trait or returned by `SeqRef`. `WithContext` is read from the expanded root options, so it may sit inside a trait, and it overrides the `ctx` passed to `InsertOneE`, `InsertManyE`, or `Plan.InsertE`.

Every record owns the values it is built from. A slice, map, or pointer passed to `Set` is copied for each record, including each record of an `InsertMany` batch, and a nullable `*T` foreign key receives its own copy of the parent's key, so an `Insert` callback that mutates them changes neither the caller's value nor another record.

With a pointer blueprint (`Blueprint[*Model]`, as generated for ent), the package-level `With` / `Generate` / `GenerateE` options take `*Model` and mutate the record in place. The `Builder` methods of the same name take `**Model` instead, because they are parameterized on the blueprint type itself; use `Apply` with the package-level option when you want the `*Model` form. `Root()` and `NodeAs` hand back the inserted pointer, and `Cleanup` deletes each record as it was when its `Insert` returned, so mutating that pointer afterwards, or from a later `Insert`, does not change which rows are removed.

## Relationship Patterns

- `BelongsTo`: insert required parent rows and write the referenced parent field into the child
- `HasMany`: insert children automatically from a parent blueprint using `Count`
- `ManyToMany`: create related rows and join-table rows together
- Composite keys: use `PKFields`, `LocalFields`, `RefFields`, and `RemoteFields`
- Non-primary references: set `RefField` or `RefFields` on `BelongsTo` when the foreign key references a unique parent field other than its primary key (for example, `CountryCode` referencing `Country.Code`)
- Matching field types: a referenced parent field of type `T` can populate a child FK of type `T` or nullable `*T`. No other implicit conversion or pointer unwrapping is performed. Named and underlying types (for example, `type UserID int64` and `int64`) remain distinct
- Self/mutual references (e.g. `employees.manager_id → employees.id`, or `A ↔ B`) must be `Optional`: a *required* reference back to a blueprint already being expanded has no finite expansion and is rejected with `ErrCycleDetected`. A `HasMany` child bound back to the parent that created it is not such a reference: `Employee → Department → HasMany Employee` inserts the department's employees with their `DepartmentID` set to that department. Because that binding is structural, a `Use` or `Ref` on the child's back-reference (for example `Ref("employees", Use("department", d))`) is rejected with `ErrInvalidOption` instead of being ignored

The execution model and graph expansion rules are documented in [ARCHITECTURE.md](../ARCHITECTURE.md).

## SQL Integration

seedling does not generate SQL at runtime. Your blueprint owns the `Insert` and optional `Delete` callbacks, so the library works with any DB abstraction that your code already uses.

- sqlc: map `Insert` callbacks to generated query methods. Prefer `seedling-gen sqlc --config ...` for automatic setup
- `database/sql`: pass `*sql.DB` or `*sql.Tx`
- pgx: pass your pool or transaction handle, or use `github.com/mhiro2/seedling/seedlingpgx` for rollback-on-cleanup helpers
- GORM: use the `gorm` subcommand to generate blueprints with `gorm.DB`-based Insert/Delete callbacks
- ent: generate the Ent client first, then use the `ent` subcommand to generate `Blueprint[*ent.X]` definitions with fluent builder Insert/Delete callbacks. The command resolves field names, mixin fields, custom entc acronyms, setter parameter types, and ID types from the package named by `--import-path`, and rejects missing or incompatible generated signatures. A singular edge (`Unique` or `Required`) must expose its FK with `.Field(...)` on whichever schema holds the column, and the opposite side of a one-to-one needs nothing; nillable fields use the generated `SetNillableX` builder methods, while optional non-nillable foreign keys are left unset when their value is zero
- Atlas HCL: use the `atlas` subcommand to generate blueprints from Atlas schema definitions

When you use `database/sql`, [`WithTx`](https://pkg.go.dev/github.com/mhiro2/seedling#WithTx) is the easiest way to get a rollback-on-cleanup transaction. [`NewTestSession`](https://pkg.go.dev/github.com/mhiro2/seedling#NewTestSession) offers the same with registry binding and custom `sql.TxOptions`. For pgx, use [`seedlingpgx.WithTx`](https://pkg.go.dev/github.com/mhiro2/seedling/seedlingpgx#WithTx) or [`seedlingpgx.NewTestSession`](https://pkg.go.dev/github.com/mhiro2/seedling/seedlingpgx#NewTestSession).

## Debugging And Cleanup

- `Plan.DebugString`: inspect the dependency tree before inserts
- `Plan.DryRunString`: inspect insert order and FK assignments without executing inserts
- `Result.DebugString`: inspect inserted nodes with primary-key values
- `Result.Node(name)`: returns the lexicographically smallest matching node ID when multiple nodes share the same blueprint name
- `Result.Nodes(name)`: returns all matching nodes in node ID order
- `Result.Cleanup` / `CleanupE`: delete inserted rows in reverse dependency order when transaction rollback is not available. Every blueprint in the inserted graph must define `Delete` (records supplied through `Use` are skipped); the callbacks are validated up front, so a missing one fails with `ErrDeleteNotDefined` before any row is deleted
- `BatchResult.DebugString`: inspect the full batch execution graph with primary-key values
- `BatchResult.Node(name)`: searches across the full batch, so use it only when cross-root ambiguity is acceptable
- `BatchResult.NodeAt(rootIndex, name)` / `NodesForRoot(rootIndex, name)`: inspect one root, its shared ancestors, and the records generated under those ancestors (such as their `HasMany` children) without mixing in sibling roots
- `BatchResult.Cleanup` / `CleanupE`: delete rows inserted by `InsertManyE`; the same `Delete` requirement applies, and cleanup is fail-fast and stops at the first delete error

## CLI

[`seedling-gen`](../cmd/seedling-gen) generates model and blueprint skeletons from multiple input sources.

Generated blueprints now include deterministic `Defaults` for common scalar fields such as strings, numbers, booleans, byte slices, and `time.Time`. Primary keys (including the untagged `ID` field GORM treats as the key by convention), relation FK fields, and unsupported custom types stay untouched so relation resolution and application-specific constraints remain explicit. Defaults for fields promoted from an embedded `gorm.Model` are written through the embedded struct (`Model: gorm.Model{CreatedAt: ...}`), and the `time` import is added only when generated code actually references it.

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

# ent schemas: parses Fields/Edges and verifies the current generated Ent package
seedling-gen ent --dir ./ent/schema --import-path github.com/you/app/ent --pkg blueprints

# Atlas HCL: parses Atlas schema file
seedling-gen atlas --pkg blueprints schema.hcl
```

Only some adapters can emit a `Delete` callback, which `Result.Cleanup` requires on every blueprint in the graph: `gorm` and `ent` always generate one, `sqlc` generates one when the queries define a unique `Delete<Table>` / `Delete<Model>` query taking the primary key, and `sql` / `atlas` generate none because they see only DDL. Add `Delete` by hand when you rely on `Cleanup` instead of `WithTx`.

All subcommands support `--pkg` (generated package name) and `--out` (output file path). The `sql` and `sqlc` subcommands also support `--dialect` (`auto`, `postgres`, `mysql`, `sqlite`). The dialect selects how string literals are read and what a primary key implies:

- `postgres` recognizes dollar-quoted strings (`$$...$$`, `$tag$...$tag$`), `E'...'` escape strings, and nested block comments; `mysql` treats a backslash inside a string literal as an escape and `#` as a line comment; `sqlite` uses standard quoting only; `auto` follows `postgres` and also reads a `#` that starts a line as a comment. Comments, string literals, and quoted identifiers never count as schema structure, so a `CREATE TABLE` or `NOT NULL` inside them is ignored
- A primary key column is `NOT NULL`, so a column that is both primary key and foreign key yields a required relation. Under `sqlite` this holds only for an `INTEGER PRIMARY KEY` rowid alias (not declared `DESC` inline) and for `WITHOUT ROWID` tables, following SQLite's own rule

All subcommands also support diagnostic output modes:

- `--explain`: print the parsed schema/model metadata plus the inferred blueprint relations instead of generated Go code
- `--json`: print the same diagnostic report as JSON, which is useful for tooling or CI checks

Both diagnostic flags report what they resolved even when generation would fail. Anything unresolved is listed under `Unresolved` (`warnings` in JSON) and the command exits non-zero, so the report stays readable while CI still catches the failure.

The `sqlc` subcommand has two input modes:

- `--config`: read `sqlc.yaml` and auto-resolve schema files, output directory, and Go import path
  - Supports sqlc config v1 (`packages`) and v2 (`sql`). `schema` may be a single path or a list; directories are expanded to their `*.sql` files in lexical order. The Go import path is derived from the nearest `go.mod` module path plus the output directory.
  - The config must contain exactly one Go-generating entry (`gen.go` in v2, one `packages` item in v1). Multiple Go entries, a missing `gen.go.out`, an unknown `version`, or malformed YAML are reported as errors rather than guessed; split multi-package configs into separate files and run `seedling-gen` once per file.
- `--dir` + `--import-path` + `<schema.sql>`: manually point at generated sqlc Go files and the schema DDL

When `--out` is specified, the output is written atomically via a temporary file so that a generation failure never leaves a partial file on disk.

The `ent` subcommand requires the package at `--import-path` to exist and compile. Run Ent code generation before `seedling-gen ent`; this lets seedling-gen use the exact generated names even when the project configures custom acronyms with entc. Generated mixin fields are included in the blueprint and its Insert callback so required mixin values are not silently omitted.

Treat generated `Defaults` as ergonomic starters, not full fake-data generation. If a test needs unique values or domain-specific shapes, override them with `Set`, `With`, or `Generate`.

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
- [quickstart](../examples/quickstart) -- generated-style `NewRegistry()` / `RegisterBlueprints(reg)` flow that matches the README Quick Start
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
