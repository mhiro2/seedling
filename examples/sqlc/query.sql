-- name: InsertOrganization :one
INSERT INTO organizations (name)
VALUES ($1)
RETURNING id, name;

-- name: InsertMember :one
INSERT INTO members (organization_id, name, email)
VALUES ($1, $2, $3)
RETURNING id, organization_id, name, email;
