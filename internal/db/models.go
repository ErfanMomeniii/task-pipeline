package db

// Task represents a row in the tasks table (sqlc generated).
type Task struct {
	ID             int64   `json:"id"`
	Type           int32   `json:"type"`
	Value          int32   `json:"value"`
	State          string  `json:"state"`
	CreationTime   float64 `json:"creation_time"`
	LastUpdateTime float64 `json:"last_update_time"`
}
