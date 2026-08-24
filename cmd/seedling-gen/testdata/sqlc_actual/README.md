# sqlc signature fixture

The `generated` directory is checked-in output from sqlc v1.30.0:

```sh
sqlc generate -f sqlc.yaml
```

Its configuration enables DB method arguments, pointer parameter structs, and pointer result structs, and renames the `User` model to `AccountRecord`. The queries cover both scalar and struct parameters, a non-primary-key foreign key, and a composite delete. The generator lifecycle test compiles seedling's output against these files and executes Insert/Delete callbacks through a recording `database/sql` driver.
