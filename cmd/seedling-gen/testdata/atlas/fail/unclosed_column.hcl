table "users" {
  column "id" {
    type = int
    null = false

  primary_key {
    columns = [column.id]
  }
}
