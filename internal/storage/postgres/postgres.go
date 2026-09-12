package postgres

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
	"github.com/starhui-dev/urbino/migrations"
)

const ExpectedSchemaVersion int64 = 2

type PoolOptions struct {
	MaxConns         int32
	MinConns         int32
	StatementTimeout time.Duration
	LockTimeout      time.Duration
	ConnectTimeout   time.Duration
}

func DefaultPoolOptions() PoolOptions {
	return PoolOptions{MaxConns: 10, StatementTimeout: 30 * time.Second, LockTimeout: 5 * time.Second, ConnectTimeout: 10 * time.Second}
}

// RedactDSN returns a display-safe DSN. It never returns a password, passfile or token.
func RedactDSN(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid-dsn>"
	}
	if u.User != nil {
		u.User = url.User(u.User.Username())
	}
	q := u.Query()
	for _, k := range []string{"password", "passfile", "sslpassword", "token"} {
		if q.Has(k) {
			q.Set(k, "[REDACTED]")
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func validateDSN(raw string) error {
	s := strings.TrimSpace(raw)
	if s == "" || (!strings.HasPrefix(s, "postgres://") && !strings.HasPrefix(s, "postgresql://")) {
		return errors.New("database DSN must be a postgres URL")
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" || u.Port() == "" || u.User == nil || u.User.Username() == "" || u.Path == "" || strings.Trim(u.Path, "/") == "" {
		return errors.New("database DSN must include explicit host, port, user and database")
	}
	if password, ok := u.User.Password(); !ok || password == "" {
		return errors.New("database DSN must include an explicit password")
	}
	for _, k := range []string{"service", "servicefile", "passfile", "sslkey", "sslcert", "sslrootcert", "sslpassword", "options"} {
		if u.Query().Get(k) != "" {
			return fmt.Errorf("database DSN option %q is not allowed", k)
		}
	}
	return nil
}

func ParsePoolConfig(raw string, opts PoolOptions) (*pgxpool.Config, error) {
	if err := validateDSN(raw); err != nil {
		return nil, err
	}
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		switch strings.ToUpper(name) {
		case "PGSERVICE", "PGSERVICEFILE", "PGPASSFILE", "PGHOST", "PGHOSTADDR", "PGPORT", "PGDATABASE", "PGUSER", "PGPASSWORD", "PGOPTIONS", "PGSSLMODE", "PGTARGETSESSIONATTRS":
			return nil, errors.New("database connection refuses implicit PG environment settings")
		}
	}
	if opts == (PoolOptions{}) {
		opts = DefaultPoolOptions()
	}
	if opts.MaxConns <= 0 || opts.MaxConns > 1000 || opts.MinConns < 0 || opts.MinConns > opts.MaxConns || opts.StatementTimeout <= 0 || opts.LockTimeout <= 0 || opts.ConnectTimeout <= 0 {
		return nil, errors.New("invalid database pool limits")
	}
	c, err := pgxpool.ParseConfig(raw)
	if err != nil {
		return nil, errors.New("invalid database DSN")
	}
	c.MaxConns, c.MinConns = opts.MaxConns, opts.MinConns
	c.ConnConfig.ConnectTimeout = opts.ConnectTimeout
	if c.ConnConfig.RuntimeParams == nil {
		c.ConnConfig.RuntimeParams = map[string]string{}
	}
	c.ConnConfig.RuntimeParams["statement_timeout"] = fmt.Sprintf("%d", opts.StatementTimeout/time.Millisecond)
	c.ConnConfig.RuntimeParams["lock_timeout"] = fmt.Sprintf("%d", opts.LockTimeout/time.Millisecond)
	c.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = fmt.Sprintf("%d", opts.StatementTimeout/time.Millisecond)
	c.ConnConfig.RuntimeParams["application_name"] = "urbino"
	c.ConnConfig.RuntimeParams["timezone"] = "UTC"
	return c, nil
}

func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	return OpenWithOptions(ctx, dsn, DefaultPoolOptions())
}
func OpenWithOptions(ctx context.Context, dsn string, opts PoolOptions) (*pgxpool.Pool, error) {
	c, err := ParsePoolConfig(dsn, opts)
	if err != nil {
		return nil, err
	}
	p, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		return nil, errors.New("database pool creation failed")
	}
	if err := p.Ping(ctx); err != nil {
		p.Close()
		return nil, errors.New("database unavailable")
	}
	if err := CheckSchema(ctx, p); err != nil {
		p.Close()
		return nil, err
	}
	return p, nil
}

type QueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func CheckSchema(ctx context.Context, db QueryRower) error {
	var got int64
	if err := db.QueryRow(ctx, `SELECT version FROM urbino.schema_version WHERE singleton = TRUE`).Scan(&got); err != nil {
		return errors.New("database schema compatibility check failed")
	}
	if got != ExpectedSchemaVersion {
		return fmt.Errorf("database schema version %d is incompatible", got)
	}
	return nil
}

// Migrate performs explicit, serialized migrations. It never runs as part of Open.
func Migrate(ctx context.Context, dsn string) error {
	if err := validateDSN(dsn); err != nil {
		return err
	}
	c, err := ParsePoolConfig(dsn, PoolOptions{MaxConns: 1, StatementTimeout: 2 * time.Minute, LockTimeout: 5 * time.Second, ConnectTimeout: 10 * time.Second})
	if err != nil {
		return err
	}
	db := stdlib.OpenDB(*c.ConnConfig)
	defer db.Close()
	locker, err := lock.NewPostgresSessionLocker(lock.WithLockTimeout(1, 5))
	if err != nil {
		return errors.New("migration lock setup failed")
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS, goose.WithTableName("urbino_goose_db_version"), goose.WithSessionLocker(locker))
	if err != nil {
		return errors.New("migration provider setup failed")
	}
	defer p.Close()
	if _, err := p.Up(ctx); err != nil {
		return errors.New("database migration failed")
	}
	return nil
}

type TxFunc func(pgx.Tx) error

func RunTx(ctx context.Context, pool interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}, opts pgx.TxOptions, maxAttempts int, fn TxFunc) error {
	if maxAttempts < 1 || maxAttempts > 10 || fn == nil {
		return errors.New("invalid transaction retry options")
	}
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		tx, err := pool.BeginTx(ctx, opts)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !retryable(err) || attempt == maxAttempts {
				return errors.New("database transaction failed")
			}
			if err := retryBackoff(ctx, attempt); err != nil {
				return err
			}
			continue
		}
		var panicValue any
		func() {
			defer func() {
				panicValue = recover()
			}()
			err = fn(tx)
		}()
		if panicValue != nil {
			_ = rollbackTx(ctx, tx)
			panic(panicValue)
		}
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			if rollbackErr := rollbackTx(ctx, tx); rollbackErr != nil {
				return errors.New("database transaction rollback failed")
			}
		}
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !retryable(err) || attempt == maxAttempts {
			return errors.New("database transaction failed")
		}
		if err := retryBackoff(ctx, attempt); err != nil {
			return err
		}
	}
	return errors.New("database transaction failed")
}

func rollbackTx(ctx context.Context, tx pgx.Tx) error {
	// A cancelled request context must not prevent the rollback itself.
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return tx.Rollback(rollbackCtx)
}

func retryBackoff(ctx context.Context, attempt int) error {
	d := time.Duration(10+rand.Intn(20)) * time.Millisecond * time.Duration(attempt)
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func retryable(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && (pe.Code == "40001" || pe.Code == "40P01")
}
