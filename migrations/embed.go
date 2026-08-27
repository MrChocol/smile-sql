// Package migrations embeds SQL migration files for use at startup.
package migrations

import _ "embed"

// InitSQL holds the contents of 0001_init.sql, embedded at compile time.
//
//go:embed 0001_init.sql
var InitSQL string
