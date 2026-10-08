package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Migration struct {
	Version int64
	Name    string
	SQL     string
}

type MigrationStatus struct {
	Migration
	AppliedAt *time.Time
}

func LoadMigrations() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	migrations := make([]Migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version, err := migrationVersion(entry.Name())
		if err != nil {
			return nil, err
		}
		content, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, Migration{Version: version, Name: entry.Name(), SQL: string(content)})
	}
	sort.Slice(migrations, func(left, right int) bool { return migrations[left].Version < migrations[right].Version })
	return migrations, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
        version bigint PRIMARY KEY, name varchar(160) NOT NULL,
        applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	migrations, err := LoadMigrations()
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		if err := applyMigration(ctx, pool, migration); err != nil {
			return err
		}
	}
	return nil
}

func Status(ctx context.Context, pool *pgxpool.Pool) ([]MigrationStatus, error) {
	migrations, err := LoadMigrations()
	if err != nil {
		return nil, err
	}
	statuses := make([]MigrationStatus, 0, len(migrations))
	for _, migration := range migrations {
		var appliedAt time.Time
		err := pool.QueryRow(ctx, "SELECT applied_at FROM schema_migrations WHERE version=$1", migration.Version).Scan(&appliedAt)
		status := MigrationStatus{Migration: migration}
		if err == nil {
			status.AppliedAt = &appliedAt
		} else if err != pgx.ErrNoRows {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func applyMigration(ctx context.Context, pool *pgxpool.Pool, migration Migration) error {
	return WithinTx(ctx, pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)", migration.Version).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return nil
		}
		if _, err := tx.Conn().Exec(ctx, migration.SQL, pgx.QueryExecModeSimpleProtocol); err != nil {
			return fmt.Errorf("执行 migration %s 失败: %w", migration.Name, err)
		}
		_, err := tx.Exec(ctx, "INSERT INTO schema_migrations(version,name) VALUES($1,$2)", migration.Version, migration.Name)
		return err
	})
}

func migrationVersion(name string) (int64, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("migration 文件名缺少版本: %s", name)
	}
	version, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil || version <= 0 {
		return 0, fmt.Errorf("migration 版本无效: %s", name)
	}
	return version, nil
}
