-- name: InsertTask :one
INSERT INTO tasks (type, value, state, creation_time, last_update_time)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, type, value, state, creation_time, last_update_time, comment;

-- name: UpdateTaskState :exec
UPDATE tasks
SET state = $2, last_update_time = $3
WHERE id = $1;

-- name: GetTask :one
SELECT id, type, value, state, creation_time, last_update_time, comment
FROM tasks
WHERE id = $1;

-- name: CountTasksByState :one
SELECT COUNT(*) FROM tasks WHERE state = $1;

-- name: SumValueByType :one
SELECT COALESCE(SUM(value), 0)::bigint AS total
FROM tasks
WHERE type = $1 AND state = 'done';

-- name: CountDoneByType :one
SELECT COUNT(*) FROM tasks WHERE type = $1 AND state = 'done';

-- name: CountUnprocessed :one
SELECT COUNT(*) FROM tasks WHERE state != 'done';

-- name: ListStaleTasks :many
SELECT id, type, value, state, creation_time, last_update_time
FROM tasks
WHERE state = 'received' AND last_update_time < $1
ORDER BY id
LIMIT $2;
