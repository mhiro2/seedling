# 🌱 seedling

Dependency-aware test data builder for Go and SQL databases.

[![Go Reference](https://pkg.go.dev/badge/github.com/mhiro2/seedling.svg)](https://pkg.go.dev/github.com/mhiro2/seedling)
[![CI](https://github.com/mhiro2/seedling/actions/workflows/ci.yaml/badge.svg)](https://github.com/mhiro2/seedling/actions/workflows/ci.yaml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

seedling lets tests create only the rows they need while automatically resolving foreign-key dependencies in the correct order. You provide the insert logic. seedling handles planning, FK assignment, and execution order.

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
- 🔗 Automatic FK resolution with topological insert ordering
- 🌿 Minimal graph expansion: only required ancestors are inserted
- 🔧 Type-safe per-test overrides with `Set`, `Use`, `Ref`, `With`, `When`, and `Only`
- ♻️ `WithTx` and companion helpers for auto-rollback transactions -- no manual cleanup
- 🔌 Works with sqlc, `database/sql`, pgx, GORM, or any other DB handle you own
- 📊 Supports `HasMany`, `ManyToMany`, composite keys, cleanup, dry runs, and insert logging
- 🎲 Includes deterministic fake data via [`seedling/faker`](https://pkg.go.dev/github.com/mhiro2/seedling/faker) with multi-locale support (en, ja, zh, ko, de, fr)

## 🚀 Quick Start

1. **Generate blueprints from your schema**

   ```bash
   go install github.com/mhiro2/seedling/cmd/seedling-gen@latest

   # From SQL DDL
   seedling-gen -pkg testutil -out blueprints.go schema.sql

   # Or from other sources:
   seedling-gen -sqlc-config sqlc.yaml -pkg testutil -out blueprints.go
   seedling-gen -gorm ./models -gorm-pkg github.com/you/app/models -pkg testutil
   seedling-gen -ent ./ent/schema -ent-pkg github.com/you/app/ent -pkg testutil
   seedling-gen -atlas schema.hcl -pkg testutil
   ```

   This generates struct types, `RegisterBlueprints()`, relations, and Insert stubs. Fill in the `// TODO` callbacks with your DB logic:

   ```go
   Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
       return insertCompany(ctx, db, v) // your DB call
   },
   ```

2. **Use it in tests**

   ```go
   func TestUser(t *testing.T) {
       tx := seedling.WithTx(t, db) // auto-rollback at cleanup

       result := seedling.InsertOne[User](t, tx)
       user := result.Root()

       if user.ID == 0 {
           t.Fatal("expected user ID to be set")
       }
       if user.CompanyID == 0 {
           t.Fatal("expected company to be inserted automatically")
       }
   }
   ```

3. **Override only what the test cares about**

   ```go
   func TestNamedUser(t *testing.T) {
       tx := seedling.WithTx(t, db)
       company := seedling.InsertOne[Company](t, tx).Root()

       result := seedling.InsertOne[User](t, tx,
           seedling.Set("Name", "alice"),
           seedling.Use("company", company),
       )

       user := result.Root()
       _ = user
   }

   func TestTaskProject(t *testing.T) {
       tx := seedling.WithTx(t, db)

       // Only("project") inserts task + project subtree only,
       // skipping the assignee relation entirely.
       result := seedling.InsertOne[Task](t, tx,
           seedling.Only("project"),
       )
       _ = result
   }
   ```

## ⚖️ Comparison

| Tool | Main model | Strong at | Not designed for |
| --- | --- | --- | --- |
| seedling | Dependency-aware builders with DB callbacks | Per-test graph generation, FK resolution, type-safe overrides, graph inspection | Bulk loading large static fixture files |
| [go-testfixtures/testfixtures](https://github.com/go-testfixtures/testfixtures) | Fixture files loaded into DB | Stable predefined datasets for integration tests | Relation-aware per-test graph construction |
| [bluele/factory-go](https://github.com/bluele/factory-go) | In-memory object factories | Flexible object construction and traits-like composition | Planning SQL insert order across FK graphs |
| [brianvoe/gofakeit](https://github.com/brianvoe/gofakeit) | Fake data generator | Realistic random values | Database insertion orchestration or relation expansion |

## 📂 Examples

- [basic](./examples/basic): register blueprints and insert rows with automatic parent creation
- [sqlc](./examples/sqlc): wire blueprints to sqlc-generated query code
- [reuse-parent](./examples/reuse-parent): reuse existing rows with `Use`
- [custom-defaults](./examples/custom-defaults): customize values with `Set`, `With`, and `Generate`
- pgx transactions: use `github.com/mhiro2/seedling/seedlingpgx` with `pgxpool.Pool` or `*pgx.Conn`
- GORM / ent / Atlas: use `seedling-gen` with `-gorm`, `-ent`, or `-atlas` flags to generate blueprints from your existing schema definitions

## 📚 Learn More

- [Guide](./docs/guide.md) -- workflows, option reference, and integration patterns
- [Architecture](./ARCHITECTURE.md) -- internal pipeline design (planner, graph, executor)
- [pkg.go.dev API reference](https://pkg.go.dev/github.com/mhiro2/seedling) -- full type and function docs

## 📝 License

MIT
