-- name: ApiKeyExists :one
SELECT EXISTS (SELECT 1 FROM api_keys WHERE key_hash = @key_hash::bytea);

-- name: ApiKeyCreate :exec
INSERT INTO api_keys (id, project_id, name, key_hash)
VALUES (@id::uuid, @project_id::uuid, @name::text, @key_hash::bytea);

-- name: ApiKeyGetForRegister :one
-- First statement of the Register transaction. FOR SHARE makes a later revoke wait for
-- the worker this transaction creates.
SELECT id, project_id
FROM api_keys
WHERE key_hash = @key_hash::bytea
FOR SHARE;
