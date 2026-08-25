---
name: seedling-test-setup
description: Write Go tests using seedling to insert, customize, and clean up test data with blueprints.
---

# seedling test setup

Use this skill when a Go test needs to insert fixture data via seedling blueprints that are already registered.

The snippets below use the package-level helpers (`seedling.InsertOne`, `seedling.InsertMany`, `seedling.Build`, ...), which read the global registry. In real tests prefer a test-local registry: call the generated `testutil.NewRegistry()` (or `seedling.NewRegistry()` + `seedling.MustRegisterTo`) and invoke the same methods on `seedling.NewSession[T](reg)`; see "Session API" below. This matches the README Quick Start and the Guide.

## Core API

### Insert a single record

```go
result := seedling.InsertOne[User](t, db)
user := result.Root()
```

`InsertOne` resolves all required `BelongsTo` parents automatically. It fails the test on error. Use `InsertOneE` for the error-returning variant.

### Insert many records

```go
roots := seedling.InsertMany[User](t, db, 5)
```

Returns `[]User`. For richer access use `InsertManyE`, which returns `BatchResult[T]`:

```go
batch, err := seedling.InsertManyE[User](ctx, db, 5)
batch.Roots()          // []User
batch.MustRootAt(0)    // User at index 0
```

### Cleanup

`Result` and `BatchResult` both provide `Cleanup` that deletes inserted records in reverse dependency order:

```go
result := seedling.InsertOne[User](t, db)
t.Cleanup(func() { result.Cleanup(t, db) })
```

`Cleanup` uses a bounded context that remains active while test cleanup callbacks run. Its budget follows the test deadline; `seedling.WithCleanupTimeout(d)` overrides it.

`Cleanup` / `CleanupE` require a `Delete` callback on every blueprint in the inserted graph (root and all auto-inserted parents; `Use`'d records are exempt). All callbacks are checked before any row is removed, so a missing one fails with `ErrDeleteNotDefined` without deleting anything. `seedling-gen` emits `Delete` for `gorm` and `ent` always, and for `sqlc` only when the queries define a unique `Delete<Table>` / `Delete<Model>` query; `sql` and `atlas` output has no `Delete`, so write it by hand before relying on `Cleanup`.

When using transaction rollback (recommended), explicit cleanup is unnecessary.

## Customization options

### Set — override a field

```go
seedling.InsertOne[User](t, db, seedling.Set("Email", "test@example.com"))
```

### Use — reuse an existing record

```go
org := getExistingOrg()
seedling.InsertOne[User](t, db, seedling.Use("organization", org))
```

The `Use`'d record is not inserted by seedling and is skipped during cleanup.

### Ref — customize an auto-created relation

```go
seedling.InsertOne[User](t, db,
    seedling.Ref("organization", seedling.Set("Name", "Acme")),
)
```

`Ref` also activates optional relations that are skipped by default.

### Omit — skip an optional relation

```go
seedling.InsertOne[User](t, db, seedling.Omit("profile"))
```

### With — type-safe struct mutation

```go
seedling.InsertOne[User](t, db, seedling.With(func(u *User) {
    u.Active = true
}))
```

### BlueprintTrait — apply a named trait

```go
seedling.InsertOne[User](t, db, seedling.BlueprintTrait("admin"))
```

Traits are defined in the blueprint's `Traits` map. A trait expands in place, so it behaves exactly like writing its options at that position: `Set` written after `BlueprintTrait` overrides the trait, and `BlueprintTrait` written after `Set` overrides the explicit value.

### Seq — per-record sequencing in InsertMany

```go
seedling.InsertMany[User](t, db, 3,
    seedling.Seq("Email", func(i int) string {
        return fmt.Sprintf("user%d@example.com", i)
    }),
)
```

## Fluent builder

Chain options for readability:

```go
result := seedling.For[User]().
    Set("Name", "Alice").
    Ref("organization", seedling.Set("Plan", "enterprise")).
    BlueprintTrait("admin").
    Insert(t, db)
```

## Transaction rollback pattern (recommended)

### database/sql

```go
func TestUser(t *testing.T) {
    tx := seedling.WithTx(t, db) // auto-rollback via t.Cleanup
    result := seedling.InsertOne[User](t, tx)
    // test logic using result.Root()
    // no cleanup needed — tx rolls back automatically
}
```

### pgx

```go
import "github.com/mhiro2/seedling/seedlingpgx"

func TestUser(t *testing.T) {
    tx := seedlingpgx.WithTx(t, pool) // auto-rollback via t.Cleanup
    result := seedling.InsertOne[User](t, tx)
    // ...
}
```

## Accessing related records from results

```go
result := seedling.InsertOne[Task](t, db)
task := result.Root()

// By blueprint name: NodeResult exposes Name() and Value() (the inserted value as any)
node, ok := result.Node("User")
if ok {
    user := node.Value().(User)
    t.Logf("%s inserted with ID %d", node.Name(), user.ID)
}

// Type-safe extraction
user := seedling.MustNodeAs[User](result, "User")       // panics if missing or wrong type
user, ok, err := seedling.NodeAs[User](result, "User")  // ok=false when missing, err on type mismatch
```

## Session API (custom registry)

When blueprints are registered to a custom `*Registry` instead of the global one:

```go
reg := seedling.NewRegistry()
seedling.MustRegisterTo(reg, userBlueprint)

session := seedling.NewSession[User](reg)
result := session.InsertOne(t, db)
```

For pgx with a session-scoped transaction:

```go
session := seedlingpgx.NewTestSession[User](t, reg, pool, pgx.TxOptions{})
result := session.InsertOne(t, session.DB())
```

## Debugging

```go
result := seedling.InsertOne[Task](t, db)
t.Log(result.DebugString()) // prints the insertion tree

// Log FK assignments during execution
seedling.InsertOne[Task](t, db,
    seedling.WithInsertLog(func(log seedling.InsertLog) {
        t.Logf("step %d: %s (table=%s)", log.Step, log.Blueprint, log.Table)
    }),
)
```

## Plan — inspect before inserting

```go
plan := seedling.Build[Task](t)
t.Log(plan.DebugString())   // planned tree
t.Log(plan.DryRunString())  // INSERT order and FK assignments

result := plan.Insert(t, db) // execute the plan
```

## Notes

- Prefer a test-local registry (generated `NewRegistry()` or `seedling.NewRegistry()` + `MustRegisterTo`) with `seedling.NewSession[T](reg)` over the global registry and package-level helpers; a per-test registry keeps blueprint state isolated between tests and packages.
- `InsertOne` / `InsertMany` accept a `testing.TB` and fail the test on error. Use the `E` variants (`InsertOneE`, `InsertManyE`) in non-test contexts or when you need explicit error handling.
- Prefer transaction rollback over `Cleanup` for test isolation — it is faster and guarantees no leftover data.
