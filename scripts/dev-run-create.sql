WITH t AS (
    SELECT id, project_id, retry_policy, start_deadline
    FROM tasks
    WHERE app = :'app' AND name = :'task'
), r AS (
    INSERT INTO runs (id, created_at, project_id, task_id, status, input, config)
    -- The server makes Run IDs in Go; this local script builds the UUIDv7 in SQL.
    SELECT encode(set_bit(set_bit(overlay(uuid_send(gen_random_uuid())
               PLACING substring(int8send(floor(extract(epoch FROM clock_timestamp()) * 1000)::bigint) FROM 3)
               FROM 1 FOR 6), 52, 1), 53, 1), 'hex')::uuid,
           now(), project_id, id, 'queued', convert_to(:'input', 'UTF8'),
           jsonb_build_object('retry', retry_policy, 'start_deadline', start_deadline)
    FROM t
    RETURNING id, project_id, task_id
)
INSERT INTO run_queue (run_id, project_id, task_id, ready_at, start_deadline_at)
SELECT r.id, r.project_id, r.task_id, now(), now() + t.start_deadline
FROM r
JOIN t ON t.id = r.task_id
RETURNING run_id;
