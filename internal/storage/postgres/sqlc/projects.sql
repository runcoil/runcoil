-- name: BootstrapLock :exec
-- First statement of the first-start transaction: a second server booting on the same
-- empty database waits here, then finds the rows.
SELECT pg_advisory_xact_lock(hashtextextended('runcoil.bootstrap', 0));

-- name: ProjectExists :one
SELECT EXISTS (SELECT 1 FROM projects);

-- name: ProjectCreate :exec
INSERT INTO projects (id, name) VALUES (@id::uuid, @name::text);

-- name: ProjectGetByName :one
SELECT id FROM projects WHERE name = @name::text;
