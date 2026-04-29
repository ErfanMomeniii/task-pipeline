package db

import "github.com/erfanmomeniii/task-pipeline/internal/models"

// Task is a type alias so sqlc-generated code in tasks.sql.go compiles
// without modification. The canonical type lives in internal/models.
type Task = models.Task
