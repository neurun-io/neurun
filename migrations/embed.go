package migrations

import "embed"

// FS contains the forward-only PostgreSQL migrations shipped with this build.
//
//go:embed *.sql
var FS embed.FS
