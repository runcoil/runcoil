-- name: UserCreateFirstServerAdmin :execrows
INSERT INTO users (id, username, password_hash, is_server_admin)
SELECT @id::uuid, @username::text, @password_hash::text, true
WHERE NOT EXISTS (SELECT 1 FROM users)
ON CONFLICT (username) DO NOTHING;

-- name: UserGetByUsername :one
SELECT id, username, email, password_hash, is_server_admin, disabled_at
FROM users
WHERE username = @username::text;

-- name: UserGetByEmail :one
SELECT id, username, email, password_hash, is_server_admin, disabled_at
FROM users
WHERE email = @email::text;

-- name: UserUpdatePassword :exec
UPDATE users
SET password_hash = @password_hash::text
WHERE id = @id::uuid;

-- name: UserLockForUpdate :one
SELECT id
FROM users
WHERE id = @id::uuid
FOR UPDATE;
