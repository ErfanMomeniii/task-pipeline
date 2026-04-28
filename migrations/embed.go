package migrations

import "embed"

// FS contains all migration SQL files embedded at compile time.
//
//go:embed *.sql
var FS embed.FS
