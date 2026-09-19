// Package migrate implements explicit, serialized PostgreSQL migrations.
package migrate

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"example.com/urbino/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const DefaultLockKey int64 = 0x5552424e4d494752 // URBNMIGR

var migrationName = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.sql$`)

type Migration struct {
	Version  int64
	Name     string
	SQL      string
	Checksum []byte
}

func LatestVersion(migrations []Migration) int64 {
	if len(migrations) == 0 {
		return 0
	}
	return migrations[len(migrations)-1].Version
}

func LoadDir(dir string) ([]Migration, error) {
	return loadFS(os.DirFS(dir), ".")
}

func loadFS(files fs.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(files, dir)
	if err != nil {
		return nil, fmt.Errorf("migration directory unavailable")
	}
	migrations := make([]Migration, 0, len(entries))
	seen := make(map[int64]struct{})
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := migrationName.FindStringSubmatch(entry.Name())
		if match == nil {
			if strings.HasSuffix(strings.ToLower(entry.Name()), ".sql") {
				return nil, fmt.Errorf("invalid migration filename")
			}
			continue
		}
		version, _ := strconv.ParseInt(match[1], 10, 64)
		if _, ok := seen[version]; ok {
			return nil, fmt.Errorf("duplicate migration version")
		}
		data, err := fs.ReadFile(files, path.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("migration file unavailable")
		}
		if strings.TrimSpace(string(data)) == "" {
			return nil, fmt.Errorf("migration file is empty")
		}
		sum := sha256.Sum256(data)
		checksum := append([]byte(nil), sum[:]...)
		seen[version] = struct{}{}
		migrations = append(migrations, Migration{Version: version, Name: match[2], SQL: string(data), Checksum: checksum})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	for i, migration := range migrations {
		if migration.Version != int64(i+1) {
			return nil, fmt.Errorf("migration versions must form a continuous positive sequence")
		}
	}
	return migrations, nil
}

type Runner struct {
	Pool        *pgxpool.Pool
	LockKey     int64
	Application string
}

func (r Runner) Run(ctx context.Context, migrations []Migration) error {
	if r.Pool == nil {
		return errors.New("migration database is not open")
	}
	if len(migrations) == 0 {
		return errors.New("no migrations found")
	}
	lockKey := r.LockKey
	if lockKey == 0 {
		lockKey = DefaultLockKey
	}
	conn, err := r.Pool.Acquire(ctx)
	if err != nil {
		return errors.New(postgres.SafeErrorMessage(err))
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		return errors.New(postgres.SafeErrorMessage(err))
	}
	defer func() { _, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", lockKey) }()
	if err := ensureHistory(ctx, conn); err != nil {
		return errors.New(postgres.SafeErrorMessage(err))
	}
	if err := validateHistory(ctx, conn, migrations); err != nil {
		switch err.Error() {
		case "schema compatibility check failed", "schema migration history drift detected":
			return err
		default:
			return errors.New(postgres.SafeErrorMessage(err))
		}
	}
	for _, migration := range migrations {
		if err := applyOne(ctx, conn, migration); err != nil {
			return err
		}
	}
	return nil
}

func ensureHistory(ctx context.Context, conn *pgxpool.Conn) error {
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS urbino_schema_migrations (
		version BIGINT PRIMARY KEY,
		name TEXT NOT NULL,
		checksum BYTEA NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return err
	}
	var present bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'urbino_schema_migrations'
		  AND column_name = 'checksum'
	)`).Scan(&present); err != nil {
		return err
	}
	if !present {
		return errors.New("schema compatibility check failed")
	}
	return nil
}

type historyRow struct {
	Version  int64
	Name     string
	Checksum []byte
}

type historyQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readHistory(ctx context.Context, querier historyQuerier) ([]historyRow, error) {
	rows, err := querier.Query(ctx, `SELECT version, name, checksum FROM urbino_schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var history []historyRow
	for rows.Next() {
		var row historyRow
		if err := rows.Scan(&row.Version, &row.Name, &row.Checksum); err != nil {
			return nil, err
		}
		history = append(history, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return history, nil
}

func validateHistoryRows(history []historyRow, migrations []Migration) error {
	if len(history) > len(migrations) {
		return errors.New("schema compatibility check failed")
	}
	for i, row := range history {
		migration := migrations[i]
		if row.Version != migration.Version || row.Name != migration.Name || !bytesEqual(row.Checksum, migration.Checksum) {
			return errors.New("schema migration history drift detected")
		}
	}
	return nil
}

func validateHistory(ctx context.Context, conn *pgxpool.Conn, migrations []Migration) error {
	history, err := readHistory(ctx, conn)
	if err != nil {
		return err
	}
	return validateHistoryRows(history, migrations)
}

func bytesEqual(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func currentVersionConn(ctx context.Context, conn *pgxpool.Conn) (int64, error) {
	var version int64
	if err := conn.QueryRow(ctx, "SELECT COALESCE(MAX(version), 0) FROM urbino_schema_migrations").Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func applyOne(ctx context.Context, conn *pgxpool.Conn, migration Migration) error {
	var applied bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM urbino_schema_migrations WHERE version = $1)", migration.Version).Scan(&applied); err != nil {
		return errors.New(postgres.SafeErrorMessage(err))
	}
	if applied {
		return nil
	}
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return errors.New(postgres.SafeErrorMessage(err))
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()
	if _, err := tx.Exec(ctx, migration.SQL); err != nil {
		return errors.New(postgres.SafeErrorMessage(err))
	}
	if _, err := tx.Exec(ctx, "INSERT INTO urbino_schema_migrations(version, name, checksum) VALUES ($1, $2, $3)", migration.Version, migration.Name, migration.Checksum); err != nil {
		return errors.New(postgres.SafeErrorMessage(err))
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.New(postgres.SafeErrorMessage(err))
	}
	committed = true
	return nil
}

func CheckCompatibility(ctx context.Context, pool *pgxpool.Pool, migrations []Migration) error {
	if pool == nil {
		return errors.New("migration database is not open")
	}
	history, err := readHistory(ctx, pool)
	if err != nil {
		return errors.New("schema compatibility check failed")
	}
	if err := validateHistoryRows(history, migrations); err != nil {
		return err
	}
	if len(history) != len(migrations) {
		return errors.New("schema compatibility check failed")
	}
	return nil
}
