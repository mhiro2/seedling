// Package seedling is a dependency-aware test data builder for Go and SQL databases.
//
// seedling is designed for tests that need real inserted rows, but do not want
// to manually wire foreign keys across multiple tables. You define explicit
// blueprints in Go, provide the insert callbacks for your own DB layer, and
// seedling plans the dependency graph, fills foreign keys, and executes inserts
// in the correct order.
//
// # Before And After
//
// Without seedling, tests often manually create each parent row in dependency order:
//
//	func TestCreateTask(t *testing.T) {
//	    company, err := db.InsertCompany(ctx, InsertCompanyParams{Name: "acme"})
//	    if err != nil {
//	        t.Fatal(err)
//	    }
//
//	    user, err := db.InsertUser(ctx, InsertUserParams{
//	        Name: "alice", CompanyID: company.ID,
//	    })
//	    if err != nil {
//	        t.Fatal(err)
//	    }
//
//	    project, err := db.InsertProject(ctx, InsertProjectParams{
//	        Name: "renewal", CompanyID: company.ID,
//	    })
//	    if err != nil {
//	        t.Fatal(err)
//	    }
//
//	    task, err := db.InsertTask(ctx, InsertTaskParams{
//	        Title: "design", ProjectID: project.ID, AssigneeUserID: user.ID,
//	    })
//	    if err != nil {
//	        t.Fatal(err)
//	    }
//
//	    _ = task
//	}
//
// With seedling, the same test can focus on the row it actually cares about:
//
//	func TestCreateTask(t *testing.T) {
//	    result := seedling.InsertOne[Task](t, db)
//	    task := result.Root()
//	    _ = task
//	}
//
// # Quick Start
//
// Register a [Blueprint] for each model that seedling should create:
//
//	seedling.MustRegister(seedling.Blueprint[Company]{
//	    Name:    "company",
//	    Table:   "companies",
//	    PKField: "ID",
//	    Defaults: func() Company {
//	        return Company{Name: "test-company"}
//	    },
//	    Insert: func(ctx context.Context, db seedling.DBTX, v Company) (Company, error) {
//	        return insertCompany(ctx, db, v)
//	    },
//	})
//
//	seedling.MustRegister(seedling.Blueprint[User]{
//	    Name:    "user",
//	    Table:   "users",
//	    PKField: "ID",
//	    Defaults: func() User {
//	        return User{Name: "test-user"}
//	    },
//	    Relations: []seedling.Relation{
//	        {Name: "company", Kind: seedling.BelongsTo, LocalField: "CompanyID", RefBlueprint: "company"},
//	    },
//	    Insert: func(ctx context.Context, db seedling.DBTX, v User) (User, error) {
//	        return insertUser(ctx, db, v)
//	    },
//	})
//
// DBTX is intentionally opaque. Your insert callback and your call sites must
// agree on the concrete handle type passed as db.
//
// Then create rows directly in your tests:
//
//	func TestUser(t *testing.T) {
//	    result := seedling.InsertOne[User](t, db)
//	    user := result.Root()
//	    // user.ID and user.CompanyID are populated.
//	}
//
// If you want to inspect the graph before executing inserts, use [Build] or
// [BuildE], then call [Plan.Validate], [Plan.DebugString], [Plan.DryRunString],
// [Plan.Insert], or [Plan.InsertE].
//
// # Core Concepts
//
// [Blueprint]
//
// A blueprint defines how to create one model type: default field values,
// primary-key metadata, relations, and insert/delete callbacks.
//
// [Relation]
//
// Relations describe graph edges such as belongs-to, has-many, and
// many-to-many. seedling uses them to expand the graph and bind keys.
//
// [Option]
//
// Options customize a single insert/build call. Common examples are [Set],
// [Use], [Ref], [Omit], [When], and [With].
//
// [Plan] and [Result]
//
// [Build] returns a plan for inspection or validation before execution.
// Reusing a [Plan] reuses its [AfterInsert] callbacks too, so state captured by
// those closures carries across executions.
// [InsertOne] returns a [Result] so you can access the root record and any
// related inserted nodes. [InsertManyE] returns a [BatchResult] with the batch
// roots plus cleanup/debug helpers for the full execution graph, including
// root-scoped lookup helpers such as [BatchResult.NodeAt] and
// [BatchResult.NodesForRoot].
//
// [Session]
//
// A session can bind a registry or database handle across repeated calls. When
// you use [database/sql], [NewTestSession] can open a transaction and roll it
// back automatically at test cleanup time. For a lighter alternative, [WithTx]
// returns a *sql.Tx directly with automatic rollback on cleanup:
//
//	tx := seedling.WithTx(t, db)
//	result := seedling.InsertOne[User](t, tx)
//
// For pgx users, the companion package github.com/mhiro2/seedling/seedlingpgx
// provides the same pattern for pgx transactions:
//
//	tx := seedlingpgx.WithTx(t, pool)
//	result := seedling.InsertOne[User](t, tx)
//
// # Common Workflows
//
// Reuse an existing parent row with [Use]:
//
//	company := seedling.InsertOne[Company](t, db).Root()
//	user := seedling.InsertOne[User](t, db,
//	    seedling.Use("company", company),
//	).Root()
//
// Customize an auto-created relation with [Ref]. This also explicitly enables
// optional relations:
//
//	plan := seedling.Build[User](t,
//	    seedling.Ref("company", seedling.Set("Name", "renewal")),
//	)
//	result := plan.Insert(t, db)
//	user := result.Root()
//	_ = user
//
// Generate multiple rows with [InsertMany] and [Seq]:
//
//	users := seedling.InsertMany[User](t, db, 3,
//	    seedling.Seq("Email", func(i int) string {
//	        return fmt.Sprintf("user-%d@example.com", i)
//	    }),
//	)
//	_ = users
//
// Or inspect / clean up the full batch execution with [InsertManyE]:
//
//	result, err := seedling.InsertManyE[User](ctx, db, 3)
//	if err != nil {
//	    _ = result.CleanupE(ctx, db)
//	}
//	company, ok := result.NodeAt(1, "company")
//	_ = company
//	_ = ok
//	users := result.Roots()
//	_ = users
//
// [InsertMany] batch-shares auto-created belongs-to relations when the same
// relation path resolves to the same static option tree after [Seq] and
// [SeqRef] are expanded. Relation-local [Use], [With], [Generate], [When], and
// rand-driven options disable sharing for that relation.
//
// Skip unnecessary relations with [Only]:
//
//	// Only insert task + project subtree; skip assignee and other relations.
//	result := seedling.InsertOne[Task](t, db,
//	    seedling.Only("project"),
//	)
//
//	// Only also works with InsertMany and applies per root.
//	result, _ := seedling.InsertManyE[Task](ctx, db, 2,
//	    seedling.Only("project"),
//	)
//	_ = result
//
// Generate deterministic fake values with [Generate], [WithSeed], and the
// seedling/faker subpackage.
//
// # SQL Integration
//
// seedling does not generate SQL at runtime. Your blueprint owns the Insert
// and optional Delete callbacks, so the library works with any DB abstraction.
// The seedling-gen CLI can generate blueprint skeletons from multiple sources:
//
//   - SQL DDL: seedling-gen sql schema.sql
//   - sqlc config: seedling-gen sqlc --config sqlc.yaml
//   - GORM models: seedling-gen gorm --dir ./models --import-path example/models
//   - ent schemas: seedling-gen ent --dir ./ent/schema --import-path example/ent
//   - Atlas HCL: seedling-gen atlas schema.hcl
//
// # Related APIs
//
// Frequently used APIs:
//
//   - [InsertOne], [InsertOneE], [InsertMany], [InsertManyE]
//   - [Build], [BuildE]
//   - [Set], [Use], [Ref], [Omit], [When], [With], [Only]
//   - [BlueprintTrait], [InlineTrait]
//   - [Seq], [Generate], [WithRand], [WithSeed]
//   - [WithContext], [WithInsertLog]
//   - [Plan.Validate], [Plan.DebugString], [Plan.DryRunString]
//   - [WithTx], [NewTestSession]
//   - [Result.Root], [Result.DebugString], [Result.Cleanup], [Result.CleanupE]
//   - [BatchResult.Roots], [BatchResult.RootAt], [BatchResult.NodeAt], [BatchResult.NodesForRoot]
//   - [BatchResult.DebugString], [BatchResult.Cleanup], [BatchResult.CleanupE]
package seedling
