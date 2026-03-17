---
name: seedling-gen-cli
description: Generate seedling blueprint scaffolding from a Go project's schema source using seedling-gen CLI.
---

# seedling-gen CLI

Use this skill when a Go project wants seedling blueprint scaffolding.

`seedling-gen` generates:

- Go model structs for tables or entities
- `RegisterBlueprints()` scaffolding
- relation definitions such as `seedling.BelongsTo`
- project-specific `Insert` stubs with `// TODO: implement` markers

It does not infer application-specific database calls. If the user wants a working integration, generate first, then replace the TODO callbacks with the project's actual insert and delete logic.

## Input selection

Choose exactly one input source. Prefer the strongest source of truth already used by the target project.

1. `sqlc.yaml` or `sqlc.json`
   Use `-sqlc-config`.
2. GORM model directory with `gorm` struct tags
   Use `-gorm` and `-gorm-pkg`.
3. ent schema directory with `Fields()` / `Edges()`
   Use `-ent` and `-ent-pkg`.
4. Atlas HCL schema
   Use `-atlas`.
5. SQL DDL files
   Use the default mode and pass a schema file path.

Do not combine adapter flags. `seedling-gen` accepts only one adapter mode at a time.

## Workflow

1. Inspect the repository and find the primary schema source.
2. Pick the output package and file path.
3. Run `seedling-gen` with one adapter mode.
4. Review generated relation names, optional relations, and composite key handling.
5. If requested, replace TODO callbacks with the project's real database code.
6. Run formatting and tests in the target repository.

Use `-out` when writing a file. The CLI writes atomically, so failures do not leave partial output behind.

## Commands

```bash
# SQL DDL
seedling-gen -pkg testutil -out internal/testutil/blueprints/seedling_gen.go schema.sql

# sqlc config
seedling-gen -sqlc-config sqlc.yaml -pkg testutil -out internal/testutil/blueprints/seedling_gen.go

# GORM
seedling-gen -gorm ./internal/models \
  -gorm-pkg github.com/acme/app/internal/models \
  -pkg testutil \
  -out internal/testutil/blueprints/seedling_gen.go

# ent
seedling-gen -ent ./ent/schema \
  -ent-pkg github.com/acme/app/ent \
  -pkg testutil \
  -out internal/testutil/blueprints/seedling_gen.go

# Atlas HCL
seedling-gen -atlas schema.hcl -pkg testutil -out internal/testutil/blueprints/seedling_gen.go
```

## Notes

- `-pkg` sets the generated Go package name.
- `-dialect` is a validation hint and defaults to `auto` when omitted. Supported values are `auto`, `postgres`, `mysql`, and `sqlite`.
- `-gorm-pkg` and `-ent-pkg` must be full Go import paths.
- The generated file is a starting point. Agents should expect follow-up edits for callback wiring and naming cleanup.

## Troubleshooting

- **GORM model parse failure** — Ensure `-gorm-pkg` points to the full Go import path of the models package. If unexported fields or custom types cause issues, check that the package compiles independently with `go build`.
- **FK or relation not detected** — `seedling-gen` infers relations from foreign key constraints. If the schema lacks explicit FK definitions (common in MySQL or older DDL), add the missing `Relation` entries manually in the generated file.
- **Composite key mismatch** — Verify that `LocalFields` and `RemoteFields` in the generated `Relation` list the columns in the same order as the schema's composite key.
- **ent edge not mapped** — Only edges backed by a foreign key column are mapped. M2M edges via join tables require manual `ManyToManyRelation` definitions.

## Verification checklist

- The chosen adapter matches the repository's real schema source.
- The generated package path and import path are correct.
- Required relations are present and optional relations were not forced accidentally.
- Composite keys and foreign keys map to the expected local and remote fields.
- Remaining TODO callbacks are either intentionally left for the user or fully implemented.
