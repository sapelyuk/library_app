// Package migrate applies the SQL files of a service to its PostgreSQL
// database.
//
// It is intentionally small: the services of this repository need "apply every
// file once, in order, inside a transaction" and nothing else. A full migration
// tool would add a dependency tree of database drivers to every service for a
// feature that is about one hundred lines of SQL bookkeeping.
//
// The rules the runner follows:
//
//   - a migration is a single .sql file named NNN_name.sql, applied in the
//     lexical order of the numeric prefix;
//   - every file runs in its own transaction, so a failed migration leaves the
//     database exactly as it was;
//   - applied files are recorded in schema_migrations together with a checksum,
//     which turns an edit of a migration that already ran into a startup error
//     instead of a silent no-op.
package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"regexp"
	"sort"
	"strings"
)

// fileName matches the accepted naming of a migration file.
var fileName = regexp.MustCompile(`^(\d+)_.+\.sql$`)

// Plan is one migration file read from the file system.
type Plan struct {
	// Version is the numeric prefix, it defines the order.
	Version string
	// Name is the file name without the extension.
	Name string
	// SQL is the content of the file.
	SQL string
	// Checksum is the SHA-256 of SQL, hex encoded.
	Checksum string
}

// Read loads and validates every migration below dir of fsys.
func Read(fsys fs.FS, dir string) ([]Plan, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("migrate: read %s: %w", dir, err)
	}

	var plans []Plan

	for _, entry := range entries {
		if entry.IsDir() || !fileName.MatchString(entry.Name()) {
			continue
		}

		content, err := fs.ReadFile(fsys, path.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("migrate: read %s: %w", entry.Name(), err)
		}

		sqlText := strings.TrimSpace(string(content))
		if sqlText == "" {
			return nil, fmt.Errorf("migrate: %s is empty", entry.Name())
		}

		sum := sha256.Sum256([]byte(sqlText))
		version := fileName.FindStringSubmatch(entry.Name())[1]

		plans = append(plans, Plan{
			Version:  version,
			Name:     strings.TrimSuffix(entry.Name(), ".sql"),
			SQL:      sqlText,
			Checksum: hex.EncodeToString(sum[:]),
		})
	}

	sort.Slice(plans, func(i, j int) bool {
		return plans[i].Version < plans[j].Version
	})

	if err := checkOrder(plans); err != nil {
		return nil, err
	}

	return plans, nil
}

// Apply runs every migration that is not recorded yet and returns the number of
// migrations it executed. A nil logger keeps the runner silent.
func Apply(ctx context.Context, db *sql.DB, fsys fs.FS, dir string, log *slog.Logger) (int, error) {
	plans, err := Read(fsys, dir)
	if err != nil {
		return 0, err
	}

	if err := ensureTable(ctx, db); err != nil {
		return 0, err
	}

	applied, err := loadApplied(ctx, db)
	if err != nil {
		return 0, err
	}

	count := 0

	for _, plan := range plans {
		recorded, ok := applied[plan.Version]
		if ok {
			if recorded != plan.Checksum {
				return count, fmt.Errorf(
					"migrate: %s changed after it was applied (stored %s, on disk %s): add a new migration instead",
					plan.Name, short(recorded), short(plan.Checksum),
				)
			}

			continue
		}

		if err := applyOne(ctx, db, plan); err != nil {
			return count, err
		}

		count++

		if log != nil {
			log.Info("migration applied", "name", plan.Name, "checksum", short(plan.Checksum))
		}
	}

	return count, nil
}

// applyOne runs the SQL of a plan and records it, in one transaction.
//
// The file is sent as a single command string without arguments, which makes
// lib/pq use the simple query protocol: that is what allows a migration to hold
// several statements.
func applyOne(ctx context.Context, db *sql.DB, plan Plan) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migrate: begin %s: %w", plan.Name, err)
	}

	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, plan.SQL); err != nil {
		return fmt.Errorf("migrate: apply %s: %w", plan.Name, err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)`,
		plan.Version, plan.Name, plan.Checksum,
	); err != nil {
		return fmt.Errorf("migrate: record %s: %w", plan.Name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migrate: commit %s: %w", plan.Name, err)
	}

	return nil
}

func ensureTable(ctx context.Context, db *sql.DB) error {
	const statement = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT        PRIMARY KEY,
    name       TEXT        NOT NULL,
    checksum   TEXT        NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`

	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("migrate: create schema_migrations: %w", err)
	}

	return nil
}

// loadApplied returns the checksum recorded for every applied version.
func loadApplied(ctx context.Context, db *sql.DB) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT version, checksum FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("migrate: read applied migrations: %w", err)
	}

	defer rows.Close()

	applied := make(map[string]string)

	for rows.Next() {
		var version, checksum string

		if err := rows.Scan(&version, &checksum); err != nil {
			return nil, fmt.Errorf("migrate: scan applied migration: %w", err)
		}

		applied[version] = checksum
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrate: iterate applied migrations: %w", err)
	}

	return applied, nil
}

// checkOrder rejects duplicated numeric prefixes, they would make the order
// ambiguous.
func checkOrder(plans []Plan) error {
	for i := 1; i < len(plans); i++ {
		if plans[i].Version == plans[i-1].Version {
			return fmt.Errorf("migrate: %s and %s share the version %s", plans[i-1].Name, plans[i].Name, plans[i].Version)
		}
	}

	return nil
}

func short(checksum string) string {
	if len(checksum) <= 12 {
		return checksum
	}

	return checksum[:12]
}
