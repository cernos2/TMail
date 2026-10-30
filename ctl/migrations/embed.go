// Package migrations holds the database schema, applied in file-name order by "zmail migrate".
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
