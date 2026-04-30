package db

import "context"

// TaskStore defines the interface for task persistence operations.
// Both the sqlc-generated Queries and test mocks implement this interface.
type TaskStore interface {
	InsertTask(ctx context.Context, arg InsertTaskParams) (Task, error)
	UpdateTaskState(ctx context.Context, arg UpdateTaskStateParams) error
	GetTask(ctx context.Context, id int64) (Task, error)
	CountTasksByState(ctx context.Context, state string) (int64, error)
	CountUnprocessed(ctx context.Context) (int64, error)
	SumValueByType(ctx context.Context, type_ int32) (int64, error)
	CountDoneByType(ctx context.Context, type_ int32) (int64, error)
	ListStaleTasks(ctx context.Context, arg ListStaleTasksParams) ([]ListStaleTasksRow, error)
}

// Compile-time check that Queries implements TaskStore.
var _ TaskStore = (*Queries)(nil)
