// Package database provides functions to manage database migrations.
// It uses the goose library to handle SQL migrations stored in an embedded filesystem.
package database

import (
	"context"
	"database/sql"
	"embed"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// migrationsDir is the directory where the migrations are stored.
const migrationsDir = "migrations"

// Migrate runs the database migrations
func Migrate(ctx context.Context, dialect string, db *sql.DB) error {
	goose.SetBaseFS(embedMigrations)

	if err := goose.SetDialect(dialect); err != nil {
		return err
	}

	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		return err
	}

	if err := goose.VersionContext(ctx, db, migrationsDir); err != nil {
		return err
	}

	return nil
}
