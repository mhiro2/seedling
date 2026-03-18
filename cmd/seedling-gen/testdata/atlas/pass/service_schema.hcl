schema "app" {}

table "companies" {
  schema = schema.app
  comment = "customer companies"

  column "id" {
    type = bigserial
    null = false
  }

  column "slug" {
    type = text
    null = false
  }

  column "billing_email" {
    type = text
    null = false
    default = "billing@example.com"
  }

  column "created_at" {
    type = timestamptz
    null = false
    default = sql("timezone('utc', now())")
  }

  primary_key {
    columns = [column.id]
  }

  index "companies_slug_key" {
    unique  = true
    columns = [column.slug]
  }

  check "companies_slug_check" {
    expr = "char_length(slug) > 2"
  }
}

table "users" {
  schema = schema.app

  column "id" {
    type = bigserial
    null = false
  }

  column "company_id" {
    type = bigint
    null = false
  }

  column "manager_id" {
    type = bigint
    null = true
  }

  column "display_name" {
    type = text
    null = false
    default = sql("coalesce(current_setting('app.user_name', true), 'user')")
  }

  column "created_at" {
    type = timestamptz
    null = false
  }

  primary_key {
    columns = [column.id]
  }

  foreign_key "users_company_id_fkey" {
    columns     = [column.company_id]
    ref_columns = [table.companies.column.id]
    on_update   = NO_ACTION
  }

  foreign_key "users_manager_id_fkey" {
    columns     = [column.manager_id]
    ref_columns = [table.users.column.id]
  }
}
