// Package postgres contains the narrow PostgreSQL boundary used by Urbino.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrorKind is a safe, stable database error category.
type ErrorKind string

const (
	ErrorInvalidConfig ErrorKind = "invalid_config"
	ErrorUnavailable   ErrorKind = "unavailable"
	ErrorConflict      ErrorKind = "conflict"
	ErrorCanceled      ErrorKind = "canceled"
	ErrorQuery         ErrorKind = "query"
)

// Error never includes a DSN or driver error text in its public message.
type Error struct {
	Kind ErrorKind
	err  error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return "database operation failed"
}

func (e *Error) Unwrap() error { return e.err }

func (e *Error) IsKind(kind ErrorKind) bool { return e != nil && e.Kind == kind }

// Config controls a pool without accepting arbitrary client-side connection
// parameters. DSN is read from a restricted secret reference by the caller.
type Config struct {
	DSN              string
	MaxConns         int32
	MinConns         int32
	StatementTimeout time.Duration
	LockTimeout      time.Duration
	ConnectTimeout   time.Duration
}

const (
	defaultStatementTimeout = 5 * time.Second
	defaultLockTimeout      = 2 * time.Second
	defaultConnectTimeout   = 5 * time.Second
)

// DB is the application storage boundary. Migration code receives the pool
// separately so application code cannot accidentally run DDL.
type DB struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, cfg Config) (*DB, error) {
	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, &Error{Kind: ErrorInvalidConfig, err: errors.New("empty database DSN")}
	}
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, &Error{Kind: ErrorInvalidConfig, err: err}
	}
	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns > 0 {
		poolCfg.MinConns = cfg.MinConns
	}
	statementTimeout := cfg.StatementTimeout
	if statementTimeout <= 0 {
		statementTimeout = defaultStatementTimeout
	}
	lockTimeout := cfg.LockTimeout
	if lockTimeout <= 0 {
		lockTimeout = defaultLockTimeout
	}
	connectTimeout := cfg.ConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = defaultConnectTimeout
	}
	poolCfg.ConnConfig.ConnectTimeout = connectTimeout
	if poolCfg.ConnConfig.RuntimeParams == nil {
		poolCfg.ConnConfig.RuntimeParams = make(map[string]string)
	}
	poolCfg.ConnConfig.RuntimeParams["statement_timeout"] = formatMilliseconds(statementTimeout)
	poolCfg.ConnConfig.RuntimeParams["lock_timeout"] = formatMilliseconds(lockTimeout)
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, &Error{Kind: ErrorUnavailable, err: err}
	}
	return &DB{pool: pool}, nil
}

func formatMilliseconds(d time.Duration) string {
	ms := d / time.Millisecond
	if ms < 1 {
		ms = 1
	}
	return fmt.Sprintf("%dms", ms)
}

func (db *DB) Pool() *pgxpool.Pool {
	if db == nil {
		return nil
	}
	return db.pool
}

func (db *DB) Close() {
	if db != nil && db.pool != nil {
		db.pool.Close()
	}
}

func (db *DB) Ping(ctx context.Context) error {
	if db == nil || db.pool == nil {
		return &Error{Kind: ErrorUnavailable, err: errors.New("database is not open")}
	}
	if err := db.pool.Ping(ctx); err != nil {
		return classifyError(err)
	}
	return nil
}

func (db *DB) Exec(ctx context.Context, sql string, args ...any) error {
	if db == nil || db.pool == nil {
		return &Error{Kind: ErrorUnavailable, err: errors.New("database is not open")}
	}
	_, err := db.pool.Exec(ctx, sql, args...)
	if err != nil {
		return classifyError(err)
	}
	return nil
}

// WithTx executes a short transaction. The callback must not perform network
// I/O. Rollback is attempted on every non-commit path and its error is never
// allowed to replace the original safe error.
func (db *DB) WithTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	if db == nil || db.pool == nil {
		return &Error{Kind: ErrorUnavailable, err: errors.New("database is not open")}
	}
	if fn == nil {
		return &Error{Kind: ErrorInvalidConfig, err: errors.New("nil transaction callback")}
	}
	tx, err := db.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return classifyError(err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()
	if err := fn(ctx, tx); err != nil {
		return classifyError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return classifyError(err)
	}
	committed = true
	return nil
}

func classifyError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return &Error{Kind: ErrorCanceled, err: err}
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505", "23503", "23514", "23P01":
			return &Error{Kind: ErrorConflict, err: err}
		case "57014":
			return &Error{Kind: ErrorCanceled, err: err}
		}
	}
	return &Error{Kind: ErrorQuery, err: err}
}

// SafeErrorMessage is suitable for client-facing logs and responses.
func SafeErrorMessage(err error) string {
	var dbErr *Error
	if errors.As(err, &dbErr) {
		switch dbErr.Kind {
		case ErrorInvalidConfig:
			return "database configuration is invalid"
		case ErrorUnavailable:
			return "database is unavailable"
		case ErrorConflict:
			return "database constraint rejected the operation"
		case ErrorCanceled:
			return "database operation canceled or timed out"
		default:
			return "database operation failed"
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "database operation canceled or timed out"
	}
	return "database operation failed"
}
