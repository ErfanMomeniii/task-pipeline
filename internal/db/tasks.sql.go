package db

import (
	"context"
)

const countDoneByType = `-- name: CountDoneByType :one
SELECT COUNT(*) FROM tasks WHERE type = $1 AND state = 'done'
`

func (q *Queries) CountDoneByType(ctx context.Context, type_ int32) (int64, error) {
	row := q.db.QueryRow(ctx, countDoneByType, type_)
	var count int64
	err := row.Scan(&count)
	return count, err
}

const countTasksByState = `-- name: CountTasksByState :one
SELECT COUNT(*) FROM tasks WHERE state = $1
`

func (q *Queries) CountTasksByState(ctx context.Context, state string) (int64, error) {
	row := q.db.QueryRow(ctx, countTasksByState, state)
	var count int64
	err := row.Scan(&count)
	return count, err
}

const countUnprocessed = `-- name: CountUnprocessed :one
SELECT COUNT(*) FROM tasks WHERE state != 'done'
`

func (q *Queries) CountUnprocessed(ctx context.Context) (int64, error) {
	row := q.db.QueryRow(ctx, countUnprocessed)
	var count int64
	err := row.Scan(&count)
	return count, err
}

const getTask = `-- name: GetTask :one
SELECT id, type, value, state, creation_time, last_update_time
FROM tasks
WHERE id = $1
`

func (q *Queries) GetTask(ctx context.Context, id int64) (Task, error) {
	row := q.db.QueryRow(ctx, getTask, id)
	var i Task
	err := row.Scan(
		&i.ID,
		&i.Type,
		&i.Value,
		&i.State,
		&i.CreationTime,
		&i.LastUpdateTime,
	)
	return i, err
}

const insertTask = `-- name: InsertTask :one
INSERT INTO tasks (type, value, state, creation_time, last_update_time)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, type, value, state, creation_time, last_update_time
`

type InsertTaskParams struct {
	Type           int32   `json:"type"`
	Value          int32   `json:"value"`
	State          string  `json:"state"`
	CreationTime   float64 `json:"creation_time"`
	LastUpdateTime float64 `json:"last_update_time"`
}

func (q *Queries) InsertTask(ctx context.Context, arg InsertTaskParams) (Task, error) {
	row := q.db.QueryRow(ctx, insertTask,
		arg.Type,
		arg.Value,
		arg.State,
		arg.CreationTime,
		arg.LastUpdateTime,
	)
	var i Task
	err := row.Scan(
		&i.ID,
		&i.Type,
		&i.Value,
		&i.State,
		&i.CreationTime,
		&i.LastUpdateTime,
	)
	return i, err
}

const sumValueByType = `-- name: SumValueByType :one
SELECT COALESCE(SUM(value), 0)::bigint AS total
FROM tasks
WHERE type = $1 AND state = 'done'
`

func (q *Queries) SumValueByType(ctx context.Context, type_ int32) (int64, error) {
	row := q.db.QueryRow(ctx, sumValueByType, type_)
	var total int64
	err := row.Scan(&total)
	return total, err
}

const updateTaskState = `-- name: UpdateTaskState :exec
UPDATE tasks
SET state = $2, last_update_time = $3
WHERE id = $1
`

type UpdateTaskStateParams struct {
	ID             int64   `json:"id"`
	State          string  `json:"state"`
	LastUpdateTime float64 `json:"last_update_time"`
}

func (q *Queries) UpdateTaskState(ctx context.Context, arg UpdateTaskStateParams) error {
	_, err := q.db.Exec(ctx, updateTaskState, arg.ID, arg.State, arg.LastUpdateTime)
	return err
}
