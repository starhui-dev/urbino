//go:build integration

package pgtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/starhui-dev/urbino/internal/storage/postgres"
)

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	raw := os.Getenv("URBINO_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("URBINO_TEST_DATABASE_URL 未提供")
	}
	if err := ValidateEnvironment(os.Environ()); err != nil {
		t.Fatal(err)
	}
	u, err := ValidateDSN(raw)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := postgres.Migrate(ctx, u.String()); err != nil {
		t.Fatal(err)
	}
	p, err := postgres.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func testUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b[0:4]) + "-" + hex.EncodeToString(b[4:6]) + "-" + hex.EncodeToString(b[6:8]) + "-" + hex.EncodeToString(b[8:10]) + "-" + hex.EncodeToString(b[10:16])
}

func TestPostgresIntegrationInvariants(t *testing.T) {
	p := integrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	t1, t2, p1, p2, u1, u2 := testUUID(t), testUUID(t), testUUID(t), testUUID(t), testUUID(t), testUUID(t)
	for _, args := range [][2]string{{t1, "A"}, {t2, "B"}} {
		if _, err := p.Exec(ctx, "INSERT INTO urbino.tenants(id,name,status,currency,policy_version) VALUES ($1,$2,'active','USD','v1')", args[0], args[1]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.Exec(ctx, "INSERT INTO urbino.projects(tenant_id,id,name,status) VALUES ($1,$2,'p1','active'),($3,$4,'p2','active')", t1, p1, t2, p2); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "INSERT INTO urbino.users(tenant_id,id,display_name,status) VALUES ($1,$2,'u1','active'),($3,$4,'u2','active')", t1, u1, t2, u2); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "INSERT INTO urbino.project_members(tenant_id,project_id,user_id,role) VALUES ($1,$2,$1::uuid,'member')", t1, p1); err == nil {
		t.Fatal("invalid user reference unexpectedly accepted")
	}
	if _, err := p.Exec(ctx, "INSERT INTO urbino.project_members(tenant_id,project_id,user_id,role) VALUES ($1,$2,$3,'member')", t1, p1, u1); err != nil {
		t.Fatal(err)
	}
	rid := testUUID(t)
	if _, err := p.Exec(ctx, "INSERT INTO urbino.requests(id,tenant_id,project_id,request_id,body_digest,model,protocol,status,dispatch_status,usage_status,started_at) VALUES ($1,$2,$3,'r',decode('aa','hex'),'m','x','started','pending','unknown',now())", rid, t1, p1); err != nil {
		t.Fatal(err)
	}
	aid := testUUID(t)
	if _, err := p.Exec(ctx, "INSERT INTO urbino.request_attempts(id,request_id,tenant_id,attempt_no,dispatch_phase,result_class,status) VALUES ($1,$2,$3,1,'preflight','none','started')", aid, rid, t1); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "INSERT INTO urbino.request_attempts(id,request_id,tenant_id,attempt_no,dispatch_phase,result_class,status) VALUES ($1,$2,$3,1,'preflight','none','started')", testUUID(t), rid, t1); err == nil {
		t.Fatal("duplicate attempt unexpectedly accepted")
	}
	ue, pv := testUUID(t), testUUID(t)
	if _, err := p.Exec(ctx, "INSERT INTO urbino.usage_events(id,tenant_id,request_id,attempt_id,source,event_key,completeness) VALUES ($1,$2,$3,$4,'provider','e','known')", ue, t1, rid, aid); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "INSERT INTO urbino.price_versions(id,version,currency,effective_at,published_at) VALUES ($1,$2,'USD',now(),now())", pv, time.Now().UnixNano()); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "INSERT INTO urbino.settlements(tenant_id,request_id,price_version_id,price_snapshot,usage_event_id,amount_micros,status,created_at) VALUES ($1,$2,$3,'{}',$4,1,'posted',now())", t1, rid, pv, ue); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "INSERT INTO urbino.settlements(tenant_id,request_id,price_version_id,price_snapshot,usage_event_id,amount_micros,status,created_at) VALUES ($1,$2,$3,'{}',$4,1,'posted',now())", t1, rid, pv, ue); err == nil {
		t.Fatal("duplicate settlement unexpectedly accepted")
	}
}

func TestPostgresRunTxRollbackOnCallbackError(t *testing.T) {
	p := integrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := p.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "CREATE TEMP TABLE urbino_tx_probe (n integer)"); err != nil {
		t.Fatal(err)
	}
	err = postgres.RunTx(ctx, conn, pgx.TxOptions{}, 1, func(tx pgx.Tx) error {
		_, _ = tx.Exec(ctx, "INSERT INTO urbino_tx_probe(n) VALUES (1)")
		return errors.New("abort")
	})
	if err == nil {
		t.Fatal("callback error unexpectedly committed")
	}
	var n int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM urbino_tx_probe").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("rollback left %d rows", n)
	}
}
