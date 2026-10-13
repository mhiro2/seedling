# GORM Gen model fixture

The `model` and `query` directories contain checked-in output from gorm/gen v0.3.27:

```sh
go run ./cmd/generate
```

The generator introspects `schema.sql` through SQLite. Its configuration produces nullable scalar fields, a belongs-to association whose explicit foreign key references a non-primary-key column, and a composite primary-key model. The seedling generator lifecycle test parses the generated model package and executes Insert/Delete callbacks through an in-memory SQLite database.

The `handwritten` directory holds hand-written models in the conventional GORM style instead: an untagged `ID` primary key and an embedded `gorm.Model`. Its lifecycle test migrates them with `AutoMigrate` and inserts several rows to check that generated defaults leave keys to the database.
