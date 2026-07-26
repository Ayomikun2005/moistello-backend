package database

import "embed"

// MigrationsFS contains the SQL migration files embedded at compile time.
// The cmd/migrate binary imports this package to run migrations without
// depending on the filesystem at runtime.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
