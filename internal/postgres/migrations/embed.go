package migrations

import "embed"

// Files contains ordered SQL migrations.
//
//go:embed *.sql
var Files embed.FS
