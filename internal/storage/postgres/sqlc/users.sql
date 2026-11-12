-- name: UserCreateFirstServerAdmin :execrows
INSERT INTO users (id, username, password_hash, is_server_admin)
SELECT @id::uuid, @username::text, @password_hash::text, true
WHERE NOT EXISTS (SELECT 1 FROM users)
ON CONFLICT (username) DO NOTHING;
