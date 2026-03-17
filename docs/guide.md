# seedling Guide

This guide keeps the README short and points to the most useful workflows when you are evaluating or adopting seedling.

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

`Only` is not supported by `InsertMany`.

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

- sqlc: map `Insert` callbacks to generated query methods. Use `-sqlc-config` for automatic setup
- `database/sql`: pass `*sql.DB` or `*sql.Tx`
- pgx: pass your pool or transaction handle
- GORM: use `-gorm` to generate blueprints with `gorm.DB`-based Insert/Delete callbacks
- ent: use `-ent` to generate blueprints with ent fluent builder Insert/Delete callbacks
- Atlas HCL: use `-atlas` to generate blueprints from Atlas schema definitions

When you use `database/sql`, [`WithTx`](https://pkg.go.dev/github.com/mhiro2/seedling#WithTx) is the easiest way to get a rollback-on-cleanup transaction. [`NewTestSession`](https://pkg.go.dev/github.com/mhiro2/seedling#NewTestSession) offers the same with registry binding and custom `sql.TxOptions`.

## Debugging And Cleanup

- `Plan.DebugString`: inspect the dependency tree before inserts
- `Plan.DryRunString`: inspect insert order and FK assignments without executing inserts
- `Result.DebugString`: inspect inserted nodes with primary-key values
- `Result.Cleanup` / `CleanupE`: delete inserted rows in reverse dependency order when transaction rollback is not available

## CLI

[`seedling-gen`](../cmd/seedling-gen) generates model and blueprint skeletons from multiple input sources:

```bash
# SQL DDL (default mode)
seedling-gen -pkg blueprints schema.sql

# sqlc config: auto-resolves schema, output dir, and import path from sqlc.yaml
seedling-gen -sqlc-config sqlc.yaml -pkg blueprints

# GORM models: parses Go source with gorm struct tags
seedling-gen -gorm ./models -gorm-pkg github.com/you/app/models -pkg blueprints

# ent schemas: parses ent schema directory (Fields/Edges methods)
seedling-gen -ent ./ent/schema -ent-pkg github.com/you/app/ent -pkg blueprints

# Atlas HCL: parses Atlas schema file
seedling-gen -atlas schema.hcl -pkg blueprints
```

Only one adapter flag can be specified at a time. All modes support `-pkg` (package name) and `-out` (output file path). When `-out` is specified, the output is written atomically via a temporary file so that a generation failure never leaves a partial file on disk.

The `-dialect` flag (`auto`, `postgres`, `mysql`, `sqlite`) is a validation hint that rejects unknown dialect names. The SQL parser itself uses the same logic for all dialects, so `-dialect` does not change parsing behavior.

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

## More References

- [pkg.go.dev package docs](https://pkg.go.dev/github.com/mhiro2/seedling)
- [faker package docs](https://pkg.go.dev/github.com/mhiro2/seedling/faker)
- [basic example](../examples/basic)
- [sqlc example](../examples/sqlc)
