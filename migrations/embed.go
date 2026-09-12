package migrations

import "embed"

// FS contains immutable goose migrations shipped with the binary.
//
//go:embed *.sql
var FS embed.FS
