---
name: seedling-test-setup
description: Write Go tests using seedling to insert, customize, and clean up test data with blueprints.
---

# seedling test setup

Use this skill when a Go test needs to insert fixture data via seedling blueprints that are already registered.

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

When using transaction rollback (recommended), explicit cleanup is unnecessary.

## Customization options

### Set — override a field

```go
seedling.InsertOne[User](t, db, seedling.Set("Email", "test@example.com"))
```

### Use — reuse an existing record

```go
org := getExistingOrg()
seedling.InsertOne[User](t, db, seedling.Use("Organization", org))
```

The `Use`'d record is not inserted by seedling and is skipped during cleanup.

### Ref — customize an auto-created relation

```go
seedling.InsertOne[User](t, db,
    seedling.Ref("Organization", seedling.Set("Name", "Acme")),
)
```

`Ref` also activates optional relations that are skipped by default.

### Omit — skip an optional relation

```go
seedling.InsertOne[User](t, db, seedling.Omit("Profile"))
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

Traits are defined in the blueprint's `Traits` map.

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
    Ref("Organization", seedling.Set("Plan", "enterprise")).
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

// By blueprint name
node, ok := result.Node("User")
userID := node.FieldByName("ID")

// Type-safe extraction
user := seedling.MustNodeAs[User](result, "User")
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

- Always register blueprints before calling `InsertOne` / `InsertMany`. If using the global registry, call `seedling.ResetRegistry()` in test setup to avoid cross-test leaks.
- `InsertOne` / `InsertMany` accept a `testing.TB` and fail the test on error. Use the `E` variants (`InsertOneE`, `InsertManyE`) in non-test contexts or when you need explicit error handling.
- Prefer transaction rollback over `Cleanup` for test isolation — it is faster and guarantees no leftover data.
