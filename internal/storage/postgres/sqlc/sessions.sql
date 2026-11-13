-- name: SessionCreate :exec
INSERT INTO sessions (token_hash, user_id, expires_at)
VALUES (@token_hash::bytea, @user_id::uuid, now() + @max_lifetime::interval);

-- name: SessionGetUser :one
SELECT u.id, u.username, u.email, u.password_hash, u.is_server_admin, s.last_seen_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = @token_hash::bytea
  AND s.expires_at > now()
  AND s.last_seen_at > now() - @max_inactive::interval
  AND u.disabled_at IS NULL;

-- name: SessionTouch :exec
UPDATE sessions
SET last_seen_at = now()
WHERE token_hash = @token_hash::bytea;

-- name: SessionDelete :exec
DELETE FROM sessions
WHERE token_hash = @token_hash::bytea;

-- name: SessionDeleteOthers :exec
DELETE FROM sessions
WHERE user_id = @user_id::uuid
  AND token_hash <> @token_hash::bytea;

-- name: SessionDeleteEnded :exec
DELETE FROM sessions
WHERE user_id = @user_id::uuid
  AND (expires_at <= now() OR last_seen_at <= now() - @max_inactive::interval);
