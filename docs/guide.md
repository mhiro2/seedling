# seedling Guide

Practical workflows and API patterns for using seedling in your tests. Start with [README Installation](../README.md#-installation) and [Quick Start](../README.md#-quick-start) if you haven't set up seedling yet.

## Core Workflows

The snippets in this guide call the package-level helpers (`seedling.InsertOne`, `seedling.InsertMany`, `seedling.Build`, ...), which read the global registry, to keep the examples short. In real tests prefer a test-local registry: create one with `seedling.NewRegistry()`, register the generated `NewBlueprints()` into it (or call `MustRegisterTo` directly), and call the same methods on `seedling.NewSession[T](reg)`, as the README Quick Start does. A per-test registry keeps blueprint state isolated between tests and packages.

### Basic inserts

Use `InsertOne` when a test needs a single root record and all required parents.

```go
result := seedling.InsertOne[Task](t, db)
task := result.Root()
```

Use `InsertMany` when a test needs multiple records of the same type. Like `InsertOne`, it returns the full result, a `BatchResult`:

```go
users := seedling.InsertMany[User](t, db, 3,
    seedling.Seq("Name", func(i int) string {
        return fmt.Sprintf("user-%d", i)
    }),
).Roots()
```

Use `InsertManyE` when an error path needs the partial result for cleanup.

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

When transaction rollback is unavailable, enable [`AutoCleanup`](#automatic-cleanup-without-transactions): the testing wrappers (`InsertOne`, `Plan.Insert`, and `InsertMany`) then record what they inserted, including a partial insert, before failing the test, and the records are deleted when the test ends. Without it, use `InsertOneE`, `Plan.InsertE`, or `InsertManyE` so an error path can call cleanup on the returned partial result.

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

`BuildMany` does the same for a batch, planning `n` records with the sharing and `Seq` expansion `InsertMany` would apply, and returns a `BatchPlan` whose `Insert` returns the `BatchResult`.

#### Plan snapshots and node limits

`Snapshot` on a `Plan`, `BatchPlan`, or `ScenarioPlan` returns a read-only `PlanSnapshot` for asserting how many and which records a fixture creates, so a test fails when a relation change makes the fixture grow:

```go
plan := seedling.Build[Task](t)
snap := plan.Snapshot()
if got := snap.Inserts(); got != 5 {
    t.Fatalf("task fixture inserts %d records, want 5:\n%s", got, plan.DebugString())
}
_ = snap.InsertsByTable() // map[string]int, e.g. "companies": 2
```

Each `PlanNode` carries its blueprint, table, relation path from its root (`task.project.company`, `department.employees[1]`, `article.tags[0].through`), why it was planned (`ReasonRoot`, `ReasonBelongsTo`, `ReasonHasMany`, `ReasonManyToMany`, `ReasonThrough`), whether it was passed with `Use` (`Provided`), whether more than one record depends on it (`Shared`, such as a parent `InsertMany` shares or a `Scenario` record linked twice), and the IDs it depends on. `Len` counts every planned record; `Inserts`, `InsertsByBlueprint`, and `InsertsByTable` count Insert callback calls — seedling cannot see how many SQL statements a callback runs.

`MaxNodes(n)` fails planning with `ErrTooManyNodes` once the plan would hold more than `n` records. It is checked while relations expand, so an unexpectedly large graph stops at the limit, and the error names the path of the record that crossed it. For `InsertMany` and `BuildMany` the limit covers the whole batch; a `Scenario` takes it from `Scenario.WithMaxNodes(n)`.

```go
seedling.InsertOne[Task](t, db, seedling.MaxNodes(5))
```

An Insert callback failure reports the same relation path through `InsertFailedError.Path()`.

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

### Sharing records across paths with Scenario

`InsertMany` shares parents only along the same relation path. When one record must sit behind several paths — the same company behind a task's project and its assignee — add each record to a `Scenario` and share it explicitly with `Link`:

```go
s := seedling.NewScenario(reg)

company := seedling.Add[Company](s)
project := seedling.Add[Project](s, seedling.Link("company", company))
assignee := seedling.Add[User](s, seedling.Link("company", company))
reviewer := seedling.Add[User](s, seedling.Link("company", company))
task := seedling.Add[Task](s,
    seedling.Link("project", project),
    seedling.Link("assignee", assignee),
)

result := s.Insert(t, db)
got := task.Get(result)
_ = reviewer.Get(result) // a second User, read back by role
```

- `Add[T]` accepts the same options as `InsertOne` and returns a typed `Node[T]` handle. Records of any blueprint can be mixed in one scenario, and seedling orders every insert
- `Link(name, node)` binds a `BelongsTo` relation to a record added earlier, instead of creating a parent. It also works under `Ref`, for example `Add[Task](s, Ref("project", Link("company", company)))`
- Nothing is shared implicitly: two `Add[User]` without `Link` get two companies. Sharing is always written down
- `node.Get(result)` returns the inserted record for that handle, so two records of the same blueprint are read back by role rather than by node ID order. `Lookup` reports whether the record is present, for example in a partial result
- `Link` conflicts with `Use`, `Ref`, `Omit`, and `When` on the same relation, and is rejected outside `Add`. `WithContext` and `WithInsertLog` are rejected in `Add`; pass the context to `Scenario.InsertE`
- `Build` / `BuildE` return a `ScenarioPlan` with `DebugString`, `DryRunString`, and `Validate`, and `InsertE` returns a partial `ScenarioResult` with `CleanupE` on failure. `AfterInsert` callbacks run after every record is inserted, in the order the records were added

### Related records per test

Configure `HasMany` and `ManyToMany` relations from the test instead of adding a blueprint or trait for every shape. Options passed to `Ref` apply to every related record, and four options narrow them:

- `Count(n)`: create `n` related records, replacing the relation's `Count`. It must be passed directly to `Ref`; `Count(0)` creates none
- `At(i, opts...)`: apply options only to the record at index `i`
- `Each(func(i int) []seedling.Option)`: derive options from each record's index
- `Through(opts...)`: configure the join record of a `ManyToMany` relation; inside `At`, only that record's join row

```go
// Three line items; the second is cancelled.
order := seedling.InsertOne[Order](t, db,
    seedling.Ref("items",
        seedling.Count(3),
        seedling.At(1, seedling.Set("Status", "cancelled")),
    ),
).Root()

// Paging boundaries without a blueprint per size.
for _, n := range []int{0, 1, 21} {
    seedling.InsertOne[Order](t, db, seedling.Ref("items", seedling.Count(n)))
}
```

Like every option, `At`, `Each`, and plain options apply in written order, so an option written after `At` overrides it for that record. `Ref` options that leave a relation with no records are rejected rather than ignored, as is an `At` index outside `Count`.

For a `ManyToMany` relation, `Use` takes a slice of existing related records and creates only the join rows, one per record. `Through` still configures those rows:

```go
member := seedling.InsertOne[Member](t, db,
    seedling.Use("roles", []Role{admin, viewer}),
    seedling.Ref("roles", seedling.At(1, seedling.Through(seedling.Set("ExpiresAt", deadline)))),
).Root()
```

The existing roles are neither inserted nor deleted by cleanup, and only `Through` options can apply to them. `HasMany` relations do not accept `Use`: attaching an existing child to another parent would need an `UPDATE`, which seedling leaves to the test.

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

### Unique and reproducible values with Scope

`Defaults` receives the `*seedling.Scope` the record is planned in. Draw values that must be unique, random, or time-based from it rather than from package-level counters or `time.Now()`:

```go
Defaults: func(s *seedling.Scope) User {
    return User{
        Email:     s.Unique("user") + "@example.com", // "user-1-k2x9q1@example.com"
        Name:      faker.New(s.Rand()).Name(),
        CreatedAt: s.Now(),
    }
},
```

- `Seq(name)` returns the next value of a named sequence, starting at 1, and `Unique(prefix)` formats it as `prefix-N`, followed by the scope's namespace when it has one. `UniqueN(prefix, n)` keeps the value within `n` bytes for a length-limited column by shortening the prefix, then dropping it, then the namespace
- `Rand()` returns a new random source seeded from the scope's seed and the number of sources drawn before it, so draws are reproducible and records do not share one stream. `Generate` options without `WithRand` / `WithSeed` draw from the same scope
- `UUID()` returns a random version 4 UUID string drawn the same way
- `Now()` is a fixed clock (2024-01-01T00:00:00Z unless configured), so time-based defaults do not change between runs

Every testing API (`InsertOne`, `InsertMany`, `Build`, `Scenario.Insert`, and their `Session`, `Builder`, and generated typed-builder forms) plans in `seedling.TestScope(t)`, one scope per test. Sequences therefore continue across every `Insert` call and every related record of the test, and the namespace and seed are derived from `t.Name()`, so two tests draw different unique values and a test draws the same values on every run as long as it plans its records in the same order. Records planned concurrently from goroutines of one test take sequence numbers and random sources in whatever order they reach the scope, so they stay unique but not reproducible. The scope is discarded when the test finishes; a cleanup function registered before the test's first insert runs after that, and a record it creates gets a separate scope that never repeats the test's values.

The error-returning APIs (`InsertOneE`, `BuildE`, `Scenario.InsertE`, ...) have no test to scope to and use one scope shared by the whole process unless given one. Pass `InScope(seedling.TestScope(t))` to make them share the test's scope, or build an explicit one with `seedling.NewScope(seedling.ScopeConfig{Namespace: "...", Seed: 42, Now: t0})` and bind it with `InScope` or `Session.WithScope`, where `InScope` overrides the session's scope. A `Scenario` takes its scope only from `Scenario.WithScope`; `InScope` on a record added to it is rejected.

Uniqueness holds within one scope. A scope never queries the database, so rows written outside it — by another process, or by an earlier run that did not roll back — can still collide; the namespace only makes collisions between tests unlikely.

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

### Automatic cleanup without transactions

A test whose HTTP server or worker reads the database over its own connection cannot see rows in an uncommitted test transaction. Commit them instead, and let `AutoCleanup` delete them when the test finishes:

```go
func TestCreateComment(t *testing.T) {
    seedling.AutoCleanup(t, db)

    task := seedling.InsertOne[Task](t, db).Root()
    users := seedling.InsertMany[User](t, db, 3).Roots()
    // ... call the server, which reads task and users over its own connection
}
```

After `AutoCleanup(t, db)`, every testing API for `t` and its subtests — `InsertOne`, `InsertMany`, `Build` / `Plan.Insert`, `BuildMany` / `BatchPlan.Insert`, `Scenario.Insert`, and their `Session`, `Builder`, and typed-builder forms — records what it inserts in the returned `CleanupTracker`. When `t` finishes, the tracker deletes through `db`, newest insert first and children before parents within each insert, so a record that `Use`s an earlier one is deleted before it.

- Records are recorded as soon as their `Insert` returns, before a failing insert fails the test, so a partial insert is deleted as well
- Records passed with `Use` belong to the caller and are never deleted
- Every blueprint a plan inserts must define `Delete`; this is checked before anything is inserted, so a missing callback fails with `ErrDeleteNotDefined` instead of leaving rows behind
- Subtest records are deleted when the test that called `AutoCleanup` finishes. Subtests are found by name, so avoid parallel sibling subtests whose own names contain `/`, such as `group/other` next to `group`. Calling `AutoCleanup` twice for one test fails the test
- Inserts running concurrently are deleted in the reverse of the order they started, so a record that uses one created by an insert still in progress is deleted first
- The error-returning APIs record only when given the tracker: `seedling.WithCleanup(tracker)` as an option, `Session.WithCleanup(tracker)`, or `Scenario.WithCleanup(tracker)`
- `CleanupTracker.CleanupE(ctx)` deletes what has been recorded so far; the rows of an insert that could not be deleted stay recorded, and a later call resumes with them without deleting the others again; errors from separate inserts are joined. The automatic cleanup reports failures with `t.Errorf` and uses the same budget as `Result.Cleanup` (`seedling.WithCleanupTimeout` caps it)

## Common Options

- `Set`: override one field by Go struct field name. Sets apply in the order written, so a later `Set` wins, including one that sets a field promoted from an embedded struct an earlier `Set` replaced
- `Ref`: customize the auto-created node behind a relation and explicitly enable an optional relation
- `Count`, `At`, `Each`, `Through`: shape the records of a `HasMany` / `ManyToMany` relation inside its `Ref` (see [Related records per test](#related-records-per-test))
- `Use`: reuse an existing record instead of inserting a new relation target, or link a slice of existing records through a `ManyToMany` relation
- `Link`: share a record added to the same `Scenario` (see [Sharing records across paths with Scenario](#sharing-records-across-paths-with-scenario))
- `Omit`: skip an optional relation
- `Only`: restrict insertion to specific relation subtrees
- `When`: expand a relation only when the current record matches a condition, including optional relations when the predicate returns true
- `With`: mutate the root struct with full type safety. Generated [typed builders](#typed-builders) extend this to field setters and relations
- `Generate` + `WithSeed` / `WithRand`: generate deterministic values for property-style tests. Without `WithSeed` / `WithRand`, `Generate` draws from the record's [Scope](#unique-and-reproducible-values-with-scope)
- `InScope`: plan the record in a specific [Scope](#unique-and-reproducible-values-with-scope)
- `WithInsertLog`: observe insert steps and FK assignments during execution

For runnable examples of these options, see [`example_test.go`](../example_test.go).

Every record owns the values it is built from. A slice, map, or pointer passed to `Set` is copied for each record, including each record of an `InsertMany` batch, and a nullable `*T` foreign key receives its own copy of the parent's key, so an `Insert` callback that mutates them changes neither the caller's value nor another record.

With a pointer blueprint (`Blueprint[*Model]`, as generated for ent), the package-level `With` / `Generate` / `GenerateE` options take `*Model` and mutate the record in place. The `Builder` methods of the same name take `**Model` instead, because they are parameterized on the blueprint type itself; use `Apply` with the package-level option when you want the `*Model` form. `Root()` and `NodeAs` hand back the inserted pointer, and `Cleanup` deletes each record as it was when its `Insert` returned, so mutating that pointer afterwards, or from a later `Insert`, does not change which rows are removed.

### Option resolution and precedence

Options resolve in one pass that every insert path shares (`InsertOne`, `InsertMany`, `Build`, and the `Builder`), so the same option list always produces the same record:

- Options apply in the order they are written. `BlueprintTrait` and `InlineTrait` expand in place, so an option written after a trait overrides it and a trait written after an option overrides that option
- Inside `InsertMany`, `Seq`, `SeqRef`, and `SeqUse` resolve per record into the `Set`, `Ref`, and `Use` they produce, so they also work inside a trait or a `SeqRef` result. Anywhere else they return `ErrInvalidOption`
- A later `Set`, `Use`, `When`, `WithContext`, `WithRand`, `InScope`, or `WithInsertLog` replaces an earlier one. Several `Ref`s to the same relation merge in written order, while `Omit`, `Only`, and `AfterInsert` accumulate
- Field values are applied in fixed stages regardless of where the options sit: blueprint `Defaults`, then `Generate`, then `Set`, then `With`
- `WithContext`, `AfterInsert`, `AfterInsertE`, `WithInsertLog`, `InScope`, and `Only` apply only to the root record. Under `Ref`, including inside a trait resolved for that relation, they return `ErrInvalidOption`
- `WithContext` may sit inside a trait, and it overrides the `ctx` passed to `InsertOneE`, `InsertManyE`, or `Plan.InsertE`
- Options under the `Ref` of a `HasMany` / `ManyToMany` relation resolve once per related record, so `At`, `Each`, and `Through` behave the same inside a trait as written directly

## Relationship Patterns

- `BelongsTo`: insert required parent rows and write the referenced parent field into the child
- `HasMany`: insert children automatically from a parent blueprint using `Count`, or per test with `Ref(name, Count(n), At(...))`
- `ManyToMany`: create related rows and join-table rows together, or link existing rows with `Use(name, []T{...})`
- Composite keys: use `PKFields`, `LocalFields`, `RefFields`, and `RemoteFields`
- Non-primary references: set `RefField` or `RefFields` on `BelongsTo` when the foreign key references a unique parent field other than its primary key (for example, `CountryCode` referencing `Country.Code`)
- Matching field types: a referenced parent field of type `T` can populate a child FK of type `T` or nullable `*T`. No other implicit conversion or pointer unwrapping is performed. Named and underlying types (for example, `type UserID int64` and `int64`) remain distinct
- Converted field types: when the FK field holds the key in another type, such as `sql.NullInt64`, `sql.Null[int64]`, `pgtype.Int8`, or a named `UserID`, set `Relation.Convert` to say how the value is converted. Keys name the FK fields (`LocalFields`, plus `RemoteFields` for `ManyToMany`); a key that names no FK field of the relation is rejected at registration. Converters run after the parent is inserted, so a zero key still becomes `{Int64: 0, Valid: true}` rather than NULL, and both the converter's input and output types are checked before the first insert. `DryRunString` shows a converted binding as `SET TenantID ← sql.NullInt64(tenants.ID)`. `seedlingpgx` provides converters for the common `pgtype` types (`Int8FK`, `Int4FK`, `Int2FK`, `TextFK`, `UUIDFK`)

```go
seedling.Relation{
    Name: "tenant", Kind: seedling.BelongsTo, LocalField: "TenantID", RefBlueprint: "tenant",
    Convert: map[string]seedling.FKConverter{
        "TenantID": seedling.ConvertFK(func(id int64) sql.NullInt64 {
            return sql.NullInt64{Int64: id, Valid: true}
        }),
    },
}
```
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

### Checking every blueprint against the database

`CheckRegistry` inserts every registered blueprint through its own `Insert` callback — once with its defaults and once with each trait — so a fixture that a migration broke fails right away instead of in whichever test uses it next:

```go
func TestFixtures(t *testing.T) {
    seedling.CheckRegistry(t, reg, func(t *testing.T) seedling.DBTX {
        return seedling.WithTx(t, db) // one rolled-back transaction per case
    },
        seedling.CheckRepeat(2),                          // insert each case twice to catch repeated unique defaults
        seedling.CheckTraits("user", "admin", "suspended"), // also check traits applied together
        seedling.ExpectFailure("user", "invalid_email"),  // a trait for negative tests must still fail its Insert
        seedling.SkipBlueprint("user_role"),               // a join blueprint used only under its relation
    )
}
```

Each case is a subtest named `blueprint/default`, `blueprint/trait`, or `blueprint/trait1+trait2`, so `go test -run 'TestFixtures/user/'` narrows it, and the database handle the function returns for that subtest isolates it. A failure names the blueprint, the traits, and the relation path of the record whose insert failed — the real database error included:

```text
--- FAIL: TestFixtures/task/default
    blueprint "task" with defaults: ... insert failed: blueprint "company" at task.project.company: null value in column "region" violates not-null constraint
```

Because the check goes through the hand-written `Insert` and `Delete` callbacks and the real schema, it catches what neither the generator nor `Plan.Validate` can see: new `NOT NULL` or `CHECK` constraints, a `UNIQUE` column whose defaults repeat (with `CheckRepeat`), and callbacks that no longer match their tables. Cases are planned in the subtest's `TestScope`; when the code cannot use a transaction, call `seedling.AutoCleanup(t, db)` in the function and return `db`. Options that name an unknown blueprint or trait fail the check, so a typo never drops a case silently.

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
- `BatchResult.Cleanup` / `CleanupE`: delete rows inserted by `InsertMany` or `InsertManyE`; the same `Delete` requirement applies, and cleanup is fail-fast and stops at the first delete error
- `AutoCleanup` / `CleanupTracker`: record every insert of a test and delete them all when it ends, instead of cleaning up each result (see [Automatic cleanup without transactions](#automatic-cleanup-without-transactions))

## CLI

[`seedling-gen`](../cmd/seedling-gen) generates model and blueprint definitions from multiple input sources.

Generated blueprints include `Defaults` for common scalar fields such as strings, numbers, booleans, byte slices, and `time.Time`, shaped by the constraints each adapter can read:

- `sql` and `sqlc`: `UNIQUE` (inline, `UNIQUE (col)`, or a single-column `CREATE UNIQUE INDEX`, including `ASC` / `DESC` / `COLLATE`), `ENUM(...)`, `CREATE TYPE ... AS ENUM`, `CHECK (col IN (...))`, `VARCHAR(n)` / `CHAR(n)`, and `UUID` / `JSON` / `JSONB` column types. Several enum and `CHECK` lists on one column are intersected
- `atlas`: unique `index` blocks over one column and `varchar(n)` / `char(n)` sizes
- `gorm`: the `unique` and `size:n` tags
- `ent`: none; only `time.Time` fields take `scope.Now()`

| Column | Generated default |
|---|---|
| unique string | `scope.Unique("user-email")`, or `scope.UniqueN("user-email", n)` for a `VARCHAR(n)` column |
| unique integer or float | `int64(scope.Seq("user-number"))` |
| enum or `CHECK (col IN (...))` | the first allowed value, or the allowed values in turn for a unique column; for a sqlc enum type, `db.UserStatus("draft")` (a nullable `NullUserStatus` is left unset) |
| `VARCHAR(n)` / `CHAR(n)` | a label cut to `n` characters |
| unique `time.Time` | `scope.Now()` plus one second per record |
| `UUID` stored as `string` | `scope.UUID()` |
| `JSON` / `JSONB` stored as `string`, `[]byte`, or `json.RawMessage` | `{}` |
| `time.Time` | `scope.Now()` |
| other strings, numbers, booleans | `"user-name"`, `1`, `true` |

`scope` is the [`*seedling.Scope`](#unique-and-reproducible-values-with-scope) the generated `Defaults` receive, so unique values keep counting across every insert of a test. Uniqueness covers rows created in that scope; the generator does not query existing rows, and composite unique constraints, arbitrary `CHECK` expressions, and unique booleans or JSON documents are not solved. Primary keys (including the untagged `ID` field GORM treats as the key by convention), relation FK fields, and unsupported custom types stay untouched so relation resolution and application-specific constraints remain explicit. Defaults for fields promoted from an embedded `gorm.Model` are written through the embedded struct (`Model: gorm.Model{CreatedAt: scope.Now(), ...}`).

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

# Migrations: files and directories are applied in order (*.down.sql skipped)
seedling-gen sql --pkg blueprints ./db/migrations

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

### Migrations and the current schema

The `sql` and `sqlc` adapters read the schema a migration history produces, not just its `CREATE TABLE` statements. Statements are applied in order across every file — the files of a directory in lexical order, skipping `*.down.sql`, as sqlc does — so later migrations change earlier tables:

- `ALTER TABLE ... ADD [COLUMN]`, `DROP [COLUMN]`, `RENAME [COLUMN] ... TO`, `RENAME TO`, and `ALTER [COLUMN] ... SET / DROP NOT NULL` or `TYPE`
- `ADD CONSTRAINT` / `DROP CONSTRAINT` for primary keys, foreign keys, `UNIQUE`, and `CHECK`, including the names PostgreSQL and MySQL generate for unnamed constraints (`users_company_id_fkey`, `users_ibfk_1`)
- MySQL `MODIFY`, `CHANGE`, `ADD ... FIRST / AFTER`, `DROP FOREIGN KEY`, `DROP INDEX`, and `RENAME TABLE`
- `DROP TABLE`, which also drops the foreign keys pointing at the table, `CREATE UNIQUE INDEX` / `DROP INDEX` / `ALTER INDEX ... RENAME TO`, and `ALTER TYPE ... ADD VALUE` / `RENAME VALUE` / `RENAME TO` and `DROP TYPE [CASCADE]` for enum types

The down section of goose, sql-migrate, dbmate, and tern migrations is ignored. Actions that do not change columns, keys, or value constraints, such as `OWNER TO`, `SET DEFAULT`, or MySQL table options, are accepted and skipped. Anything else that could change the schema — an unknown `ALTER TABLE` action, an `ALTER TABLE` on a table no earlier statement created, or a `DROP CONSTRAINT` whose name cannot be resolved — stops generation with the file and line, rather than silently producing blueprints for a schema that no longer exists:

```text
Error: db/migrations/007_partition.up.sql:3: ALTER TABLE events: unsupported action "ATTACH PARTITION ..."
```

When a migration history is beyond what the parser follows, generate from the schema of a migrated database instead. The dump tools write plain DDL in the forms above, with keys added by `ALTER TABLE`:

```bash
pg_dump --schema-only --no-owner "$DATABASE_URL" > schema.sql   # PostgreSQL
mysqldump --no-data --skip-comments app > schema.sql            # MySQL
sqlite3 app.db .schema > schema.sql                             # SQLite
seedling-gen sql --pkg blueprints schema.sql
```

### Generated and hand-written files

The generated file starts with `// Code generated by seedling-gen. DO NOT EDIT.` and is rewritten on every run, so it holds only what the schema determines:

- model structs (`sql` and `atlas` only)
- a `Blueprints` struct with one `seedling.Blueprint[T]` field per model, returned by `NewBlueprints()`
- `Name`, `Table`, primary keys, `BelongsTo` relations, and starter `Defaults`
- `Convert` entries on `BelongsTo` relations whose FK field wraps the referenced key in a nullable type: `sql.NullInt64` / `NullInt32` / `NullInt16` / `NullByte` / `NullString`, `sql.Null[T]` for a predeclared `T`, `pgtype.Int8` / `Int4` / `Int2` / `Text` / `UUID`, and `uuid.NullUUID` (`sqlc` and `gorm`). An integer key is widened only when every value fits, so an `int32` key may fill an `sql.NullInt64` but a `uint` or `int64` key never fills an `sql.NullInt16`. Other mismatches, such as a named ID type, are left for the hand-written file to set with `seedling.ConvertFK`; `Plan.Validate` and the insert preflight report them before any row is written
- an `Optional` `HasMany` relation on each parent for every `BelongsTo` that points at it, named after the plural child blueprint (`users`), or qualified by the relation name when it differs from the parent (`assignee_tasks`). These never expand by default; create children from the parent side with `Ref("users", seedling.Count(3))`
- `Insert` / `Delete` callbacks where the adapter can derive them
- `Register(reg) error` and `MustRegister(reg)`, which register every blueprint and report all failures together
- `NewFixtures(reg)` and one typed builder per model (see [Typed builders](#typed-builders))

Everything project-specific lives in a hand-written file of the same package that edits the generated blueprints before registering them:

```go
// blueprints.go (hand-written; seedling-gen never touches it)
func NewRegistry() *seedling.Registry {
    blueprints := NewBlueprints()

    blueprints.User.Insert = func(ctx context.Context, db seedling.DBTX, v User) (User, error) {
        return insertUser(ctx, db, v)
    }

    generatedDefaults := blueprints.User.Defaults
    blueprints.User.Defaults = func(s *seedling.Scope) User {
        u := generatedDefaults(s)
        u.Email = s.Unique("user") + "@example.com"
        return u
    }

    blueprints.User.Traits = map[string][]seedling.Option{
        "admin": {seedling.Set("Role", "admin")},
    }

    reg := seedling.NewRegistry()
    blueprints.MustRegister(reg)
    return reg
}
```

Wrapping the generated `Defaults` instead of replacing it keeps defaults for columns added later. Because customizations address fields by name, adding a column or relation and regenerating does not break the hand-written file. A new table appears as a new `Blueprints` field and is registered automatically; if its adapter cannot derive `Insert`, `Register` fails with `blueprint "<name>" must have an Insert function` until the hand-written file sets one. No adapter emits an `Insert` that silently succeeds without writing a row.

Add `--check` to the generation command in CI to detect a schema change whose output was not regenerated:

```bash
seedling-gen sql --pkg testutil --out testutil/blueprints_gen.go --check schema.sql
```

`--check` generates into memory, compares the result with the `--out` file byte for byte, and exits non-zero when the file is missing or differs, without writing anything. It requires `--out` and cannot be combined with `--explain` or `--json`. Imports are grouped the way `goimports` does by default. Formatters that honor the generated-code header, including `golangci-lint fmt`, leave the file alone; exclude it from any formatter that regroups imports (such as `goimports -local`), or `--check` reports a difference.

### Typed builders

The generated file also declares a typed builder per model, so field names, relation names, and the model on the other side of each relation are checked by the compiler instead of at run time:

```go
f := testutil.NewFixtures(reg)

task := f.Task().
    Title("review").
    Assignee(f.User().Name("alice")).
    Insert(t, db).
    Root()

// Children through the generated reverse relation: one builder per record.
order := f.Order().Items(f.Item(), f.Item().Status("cancelled"), f.Item()).Insert(t, db)

// Sharing in a Scenario.
s := seedling.NewScenario(reg)
company := f.Company().Add(s)
alice := f.User().Name("alice").LinkCompany(company).Add(s)
```

Each `XBuilder` is an immutable value: every method returns a new builder, so a shared base such as `base := f.User()` can be extended in several directions. A builder for a model offers:

- one setter per field, such as `Title(v string)`, for fields whose type the generator can name: predeclared types other than `any`, and pointers, slices, arrays, or maps of them, plus `time`, `database/sql`, and `encoding/json` types except for the hand-written models of the `gorm` adapter, whose package names may be aliases. Relation FK fields and fields shadowed by another field of the same name have no setter, and other types are set through `With`
- for each `BelongsTo` relation, `Assignee(UserBuilder)` to configure the created parent, `UseAssignee(User)` to bind an existing row, and `LinkAssignee(seedling.Node[User])` to bind a `Scenario` record; passing a builder of another model does not compile
- for each generated `HasMany` relation, `Items(children ...ItemBuilder)`, which creates exactly one record per builder (`Items()` creates none)
- `With(func(*Model))`, `Apply(presets ...func(XBuilder) XBuilder)` for typed reusable presets, `Trait(name)` for traits registered on the blueprint, `AfterInsert` (only on the record a terminal method inserts; a builder carrying it cannot be passed to a relation method), and `WithOptions(...seedling.Option)` as an untyped escape hatch for options such as `Seq` or `Omit`
- `Insert`, `InsertE`, `InsertMany`, `Build`, and `Add(*seedling.Scenario)`

When a field or relation name collides with one of these methods or with another generated method, relations keep the name and the colliding field gets no setter; use `With` for it. Relations added by hand to `Blueprints` have no typed method; use `WithOptions(seedling.Ref(...))`.

### Callbacks and keys

Only some adapters can emit a `Delete` callback, which `Result.Cleanup` requires on every blueprint in the graph: `gorm` and `ent` always generate one, `sqlc` generates one when the queries define a unique `Delete<Table>` / `Delete<Model>` query taking the primary key, and `sql` / `atlas` generate none because they see only DDL. Set `Delete` in the hand-written file when you rely on `Cleanup` instead of `WithTx`.

The `sql` and `atlas` adapters take a blueprint's key from the table's declared primary key, fall back to an `id` column, and leave `PKField` unset for a table with neither, so the blueprint still passes `Plan.Validate` and can be inserted. `sqlc` rejects such a table instead.

### Flags

All subcommands support `--pkg` (generated package name), `--out` (output file path), and `--check` (verify `--out` is up to date). The `sql` and `sqlc` subcommands also support `--dialect` (`auto`, `postgres`, `mysql`, `sqlite`). The dialect selects how string literals are read and what a primary key implies:

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

Treat generated `Defaults` as ergonomic starters, not full fake-data generation. They follow the single-column constraints their adapter reads (see above); for other constraints and domain-specific shapes, wrap them in the hand-written file, drawing from the `Scope` they receive, or override them per test with `Set`, `With`, or `Generate`.

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
- [quickstart](../examples/quickstart) -- seedling-gen output kept apart from a hand-written `NewRegistry()`, matching the README Quick Start
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
