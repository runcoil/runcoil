-- name: LeaseClaim :many
-- @slots rows at most per Task through (task_id, ready_at), then @slots in all. Rows
-- locked but not picked are released at COMMIT. A Run past its start deadline is never
-- claimed.
WITH picked AS (
    SELECT q.run_id
    FROM unnest(@task_ids::uuid[]) AS t(task_id)
    CROSS JOIN LATERAL (
        SELECT run_id, ready_at
        FROM run_queue
        WHERE task_id = t.task_id
          AND (start_deadline_at IS NULL OR start_deadline_at > now())
        ORDER BY ready_at
        LIMIT @slots::integer
        FOR UPDATE SKIP LOCKED
    ) q
    ORDER BY q.ready_at
    LIMIT @slots::integer
), moved AS (
    DELETE FROM run_queue
    WHERE run_id IN (SELECT run_id FROM picked)
    RETURNING run_id, project_id, task_id, retry_step_id, retry_count, lost_count
)
INSERT INTO run_leases (run_id, project_id, task_id, worker_id, retry_step_id, retry_count, lost_count)
SELECT run_id, project_id, task_id, @worker_id::uuid, retry_step_id, retry_count, lost_count
FROM moved
RETURNING run_id, lease_epoch;

-- name: LeaseReleaseUnknown :many
-- Leases the worker did not report go back to the queue, without counting as lost.
WITH orphaned AS (
    DELETE FROM run_leases
    WHERE worker_id = @worker_id::uuid AND lease_epoch <> ALL (@held_epochs::bigint[])
    RETURNING run_id, project_id, task_id, retry_step_id, retry_count, lost_count
)
INSERT INTO run_queue (run_id, project_id, task_id, ready_at, retry_step_id, retry_count, lost_count)
SELECT run_id, project_id, task_id, now(), retry_step_id, retry_count, lost_count
FROM orphaned
RETURNING run_id;

-- name: LeaseListNotHeld :many
-- The Runs the worker reports but no longer holds.
SELECT r.run_id::uuid AS run_id, r.lease_epoch::bigint AS lease_epoch
FROM (SELECT unnest(@run_ids::uuid[]) AS run_id, unnest(@lease_epochs::bigint[]) AS lease_epoch) AS r
WHERE NOT EXISTS (
    SELECT 1 FROM run_leases l
    WHERE l.run_id = r.run_id AND l.lease_epoch = r.lease_epoch AND l.worker_id = @worker_id::uuid
);

-- name: LeaseLock :one
-- First statement of every worker call on a Run.
SELECT retry_step_id, retry_count
FROM run_leases
WHERE run_id = @run_id::uuid AND lease_epoch = @lease_epoch::bigint AND worker_id = @worker_id::uuid
FOR UPDATE;

-- name: LeaseResetStepRetry :exec
UPDATE run_leases
SET retry_step_id = NULL, retry_count = 0
WHERE run_id = @run_id::uuid AND retry_step_id = @step_id::text;
