# Ent signature fixture

The `ent` directory contains checked-in output from entc v0.14.5:

```sh
go run entgo.io/ent/cmd/ent@v0.14.5 generate ./ent/schema
```

The schema exercises nillable fields, an edge backed by an explicit foreign-key field, a required field supplied by a mixin, and an unused mixin defined in the schema package, which is not a schema. The generator lifecycle test compiles seedling's output against these files and executes Insert/Delete callbacks through an in-memory SQLite database.
