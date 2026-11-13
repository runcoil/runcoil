-- name: ProjectMemberListByUser :many
SELECT p.id, p.name, m.role
FROM project_members m
JOIN projects p ON p.id = m.project_id
WHERE m.user_id = @user_id::uuid
ORDER BY p.name;

-- name: ProjectMemberGetRole :one
SELECT role
FROM project_members
WHERE project_id = @project_id::uuid
  AND user_id = @user_id::uuid;
