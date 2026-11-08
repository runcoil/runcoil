-- name: RunMarkStarted :exec
UPDATE runs
SET status = 'running', started_at = now()
WHERE id = ANY (@ids::uuid[]) AND status = 'queued';

-- name: RunListForAssign :many
SELECT r.id, t.name AS task_name, r.input, r.created_at
FROM runs r
JOIN tasks t ON t.id = r.task_id
WHERE r.id = ANY (@ids::uuid[]);

-- name: RunFinish :one
-- Ends the attempt that holds the lease. No row: the lease is not this attempt's, or the
-- Run already ended.
WITH released AS (
    DELETE FROM run_leases
    WHERE run_id = @run_id::uuid AND lease_epoch = @lease_epoch::bigint AND worker_id = @worker_id::uuid
    RETURNING run_id
)
UPDATE runs r
SET status        = @status::text,
    output        = sqlc.narg(output)::bytea,
    error_code    = sqlc.narg(error_code)::text,
    error_message = sqlc.narg(error_message)::text,
    ended_at      = now()
FROM released
WHERE r.id = released.run_id
RETURNING r.id;

-- name: RunGetStatus :one
SELECT status FROM runs WHERE id = @id::uuid;
