-- name: InsertCompany :one
INSERT INTO companies (code, spotify_url)
VALUES (sqlc.arg(code), sqlc.arg(spotify_url))
RETURNING id, code, spotify_url;

-- name: DeleteCompany :exec
DELETE FROM companies WHERE id = sqlc.arg(id);

-- name: InsertUser :one
INSERT INTO users (display_name, company_code)
VALUES (sqlc.arg(display_name), sqlc.arg(company_code))
RETURNING id, display_name, company_code;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = sqlc.arg(id);

-- name: InsertMembership :one
INSERT INTO memberships (organization_id, user_id)
VALUES (sqlc.arg(organization_id), sqlc.arg(user_id))
RETURNING organization_id, user_id;

-- name: DeleteMembership :exec
DELETE FROM memberships
WHERE organization_id = sqlc.arg(organization_id)
  AND user_id = sqlc.arg(user_id);

-- name: InsertLabel :one
INSERT INTO labels (name)
VALUES (sqlc.arg(name))
RETURNING id, name;

-- name: DeleteLabel :exec
DELETE FROM labels WHERE id = sqlc.arg(id);
