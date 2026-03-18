CREATE TYPE accounting.user_status AS ENUM ('active', 'disabled');

CREATE TABLE "accounting"."companies" (
    "id" BIGSERIAL PRIMARY KEY,
    "slug" TEXT NOT NULL UNIQUE,
    "billing_email" TEXT NOT NULL DEFAULT 'billing@example.com',
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    CONSTRAINT "companies_slug_check" CHECK (char_length("slug") > 2)
);

COMMENT ON TABLE "accounting"."companies" IS 'customer companies';

CREATE TABLE "accounting"."users" (
    "id" BIGSERIAL PRIMARY KEY,
    "company_id" BIGINT NOT NULL,
    "display_name" TEXT NOT NULL DEFAULT concat('user-', substr(md5(random()::text), 1, 8)),
    "status" accounting.user_status NOT NULL DEFAULT 'active',
    "search_name" TEXT GENERATED ALWAYS AS (lower("display_name")) STORED,
    "manager_id" BIGINT,
    "created_at" TIMESTAMPTZ NOT NULL DEFAULT (now() AT TIME ZONE 'UTC'),
    CONSTRAINT "users_company_id_fkey"
        FOREIGN KEY ("company_id")
        REFERENCES "accounting"."companies"("id"),
    CONSTRAINT "users_manager_id_fkey"
        FOREIGN KEY ("manager_id")
        REFERENCES "accounting"."users"("id"),
    CONSTRAINT "users_display_name_check"
        CHECK (char_length("display_name") > 0 AND position(' ' in "display_name") >= 0)
);

CREATE TRIGGER users_search_name_refresh
BEFORE UPDATE ON "accounting"."users"
FOR EACH ROW
EXECUTE FUNCTION accounting.refresh_search_name();
