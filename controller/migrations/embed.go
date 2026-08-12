package migrations

import "embed"

// FS contains immutable, ordered database migrations.
//
//go:embed *.sql
var FS embed.FS
