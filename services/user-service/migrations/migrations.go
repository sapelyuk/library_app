// Package migrations embeds the SQL files of the user service database.
//
// The files are applied by pkg/migrate when the service starts with
// USER_SERVICE_DB_MIGRATE=true, so a fresh environment comes up with the schema
// the binary expects instead of relying on somebody remembering to run a tool.
package migrations

import "embed"

// FS holds every *.sql file of this directory.
//
//go:embed *.sql
var FS embed.FS
