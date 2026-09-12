package postgres

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"
	"time"
)

type schemaRow struct {
	version int64
	err     error
}

func (r schemaRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	p, ok := dest[0].(*int64)
	if !ok {
		return errors.New("bad destination")
	}
	*p = r.version
	return nil
}

type schemaDB struct{ row schemaRow }

func (d schemaDB) QueryRow(context.Context, string, ...any) pgx.Row { return d.row }

func TestRedactDSN(t *testing.T) {
	raw := "postgres://user:super-secret@127.0.0.1:5432/urbino?sslpassword=secret&token=abc"
	out := RedactDSN(raw)
	if strings.Contains(out, "super-secret") || strings.Contains(out, "secret") || strings.Contains(out, "abc") {
		t.Fatalf("DSN secret leaked: %q", out)
	}
	if !strings.Contains(out, "user@127.0.0.1") {
		t.Fatalf("user/host missing: %q", out)
	}
}

func TestParsePoolConfigSetsTimeoutsAndRejectsImplicitSources(t *testing.T) {
	dsn := "postgres://user:synthetic@127.0.0.1:5432/urbino?sslmode=disable"
	c, err := ParsePoolConfig(dsn, PoolOptions{MaxConns: 2, StatementTimeout: time.Second, LockTimeout: 250 * time.Millisecond, ConnectTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if c.ConnConfig.RuntimeParams["statement_timeout"] != "1000" || c.ConnConfig.RuntimeParams["lock_timeout"] != "250" {
		t.Fatalf("timeouts not applied: %#v", c.ConnConfig.RuntimeParams)
	}
	for _, bad := range []string{"postgres://user@127.0.0.1:5432/urbino", "postgres://user:synthetic@127.0.0.1/urbino", dsn + "&service=prod", "host=127.0.0.1 dbname=urbino"} {
		if _, err := ParsePoolConfig(bad, PoolOptions{}); err == nil {
			t.Fatalf("unsafe DSN accepted: %q", bad)
		}
	}
}

func TestCheckSchemaUsesStableCompatibilityError(t *testing.T) {
	if err := CheckSchema(context.Background(), schemaDB{row: schemaRow{version: ExpectedSchemaVersion}}); err != nil {
		t.Fatal(err)
	}
	for _, row := range []schemaRow{{version: 9}, {err: errors.New("password=secret driver error")}} {
		err := CheckSchema(context.Background(), schemaDB{row: row})
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("schema error leaked or accepted: %v", err)
		}
	}
}
