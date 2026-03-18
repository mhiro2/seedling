schema "app" {}

table "regions" {
  schema = schema.app

  column "country_code" {
    type = text
    null = false
  }

  column "region_code" {
    type = text
    null = false
  }

  primary_key {
    columns = [column.country_code, column.region_code]
  }
}

table "deployments" {
  schema = schema.app

  column "id" {
    type = bigserial
    null = false
  }

  column "region_country_code" {
    type = text
    null = false
  }

  column "region_code" {
    type = text
    null = false
  }

  column "environment" {
    type = varchar(32)
    null = false
  }

  primary_key {
    columns = [column.id]
  }

  foreign_key "deployments_region_fkey" {
    columns = [
      column.region_country_code,
      column.region_code,
    ]
    ref_columns = [
      table.regions.column.country_code,
      table.regions.column.region_code,
    ]
    on_delete = CASCADE
  }

  index "deployments_unique_per_env" {
    unique = true
    columns = [
      column.region_country_code,
      column.region_code,
      column.environment,
    ]
  }
}
