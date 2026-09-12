//go:build integration

package pgtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/starhui-dev/urbino/internal/storage/postgres"
)

const runtimeRole = "urbino_runtime"

var safeDBName = regexp.MustCompile(`^urbino_test_[0-9a-f]{24}$`)

// env 只清理本次创建的临时数据库。传入 DSN 使用前先验证，绝不重置或删除原数据库。
type env struct {
	base                       *pgx.Conn
	dbName, dbDSN, runtimePass string
	roleCreated                bool
}

func newEnv(t *testing.T) *env {
	t.Helper()
	raw := os.Getenv("URBINO_TEST_DATABASE_URL")
	if raw == "" {
		t.Fatal("URBINO_TEST_DATABASE_URL 未提供；集成门禁不能静默跳过")
	}
	if err := ValidateEnvironment(os.Environ()); err != nil {
		t.Fatal(err)
	}
	u, err := ValidateDSN(raw)
	if err != nil {
		t.Fatal(err)
	}
	c, err := pgx.ParseConfig(u.String())
	if err != nil {
		t.Fatal("测试数据库配置无效")
	}
	c.ConnectTimeout = 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	base, err := pgx.ConnectConfig(ctx, c)
	if err != nil {
		t.Fatal("测试数据库不可连接")
	}
	e := &env{base: base}
	t.Cleanup(func() { e.cleanup(t) })
	if _, err := base.Exec(ctx, "SELECT pg_advisory_lock(hashtextextended('urbino-pgtest-suite', 0))"); err != nil {
		t.Fatal("无法取得测试套件锁")
	}
	if err := e.ensureRole(ctx); err != nil {
		t.Fatal(err)
	}
	e.dbName = "urbino_test_" + randomHex(t, 12)
	if !safeDBName.MatchString(e.dbName) {
		t.Fatal("生成的测试数据库名不安全")
	}
	if _, err := base.Exec(ctx, "CREATE DATABASE "+quoteIdentifier(e.dbName)+" TEMPLATE template0"); err != nil {
		t.Fatal("无法创建隔离测试数据库")
	}
	e.dbDSN = withDatabase(u, e.dbName).String()
	return e
}

func (e *env) ensureRole(ctx context.Context) error {
	var present, superuser, createRole, createDB, replication, bypass, canLogin bool
	err := e.base.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=$1),
 COALESCE((SELECT rolsuper FROM pg_roles WHERE rolname=$1),false), COALESCE((SELECT rolcreaterole FROM pg_roles WHERE rolname=$1),false),
 COALESCE((SELECT rolcreatedb FROM pg_roles WHERE rolname=$1),false), COALESCE((SELECT rolreplication FROM pg_roles WHERE rolname=$1),false),
 COALESCE((SELECT rolbypassrls FROM pg_roles WHERE rolname=$1),false), COALESCE((SELECT rolcanlogin FROM pg_roles WHERE rolname=$1),false)`, runtimeRole).
		Scan(&present, &superuser, &createRole, &createDB, &replication, &bypass, &canLogin)
	if err != nil {
		return errors.New("无法读取测试 runtime 角色")
	}
	if present {
		if superuser || createRole || createDB || replication || bypass {
			return errors.New("urbino_runtime 角色具有不安全的高权限属性")
		}
		var memberships int
		if err := e.base.QueryRow(ctx, `SELECT count(*) FROM pg_auth_members m JOIN pg_roles r ON r.oid=m.member WHERE r.rolname=$1`, runtimeRole).Scan(&memberships); err != nil {
			return errors.New("无法检查 runtime 角色继承关系")
		}
		if memberships != 0 {
			return errors.New("urbino_runtime 继承了其他角色")
		}
		if !canLogin {
			return errors.New("预先存在的 urbino_runtime 必须可登录，才能执行运行角色测试")
		}
		return nil
	}
	e.runtimePass = randomHex(nil, 24)
	if _, err := e.base.Exec(ctx, "CREATE ROLE "+quoteIdentifier(runtimeRole)+" LOGIN PASSWORD '"+e.runtimePass+"' NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS"); err != nil {
		return errors.New("无法创建隔离 runtime 角色")
	}
	e.roleCreated = true
	return nil
}

func (e *env) cleanup(t *testing.T) {
	if e.base == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if e.dbName != "" && safeDBName.MatchString(e.dbName) {
		_, _ = e.base.Exec(ctx, "DROP DATABASE "+quoteIdentifier(e.dbName)+" WITH (FORCE)")
	}
	if e.roleCreated {
		_, _ = e.base.Exec(ctx, "DROP ROLE "+quoteIdentifier(runtimeRole))
	}
	_, _ = e.base.Exec(ctx, "SELECT pg_advisory_unlock(hashtextextended('urbino-pgtest-suite', 0))")
	e.base.Close(ctx)
}

func (e *env) pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := postgres.Migrate(ctx, e.dbDSN); err != nil {
		t.Fatal(err)
	}
	p, err := postgres.Open(ctx, e.dbDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func (e *env) runtimeConn(t *testing.T) *pgx.Conn {
	t.Helper()
	if e.runtimePass == "" {
		t.Fatal("预先存在的 runtime 角色没有可安全使用的测试凭据")
	}
	u, err := url.Parse(e.dbDSN)
	if err != nil {
		t.Fatal("隔离 DSN 构造失败")
	}
	u.User = url.UserPassword(runtimeRole, e.runtimePass)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := pgx.Connect(ctx, u.String())
	if err != nil {
		t.Fatal("runtime 角色无法连接隔离数据库")
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return c
}

func assertRuntimeCannotDDL(t *testing.T, e *env, p *pgxpool.Pool, ctx context.Context) {
	t.Helper()
	if e.runtimePass != "" {
		rc := e.runtimeConn(t)
		expectCode(t, rc, ctx, "42501", "CREATE TABLE urbino.runtime_must_not_ddl(n integer)")
		return
	}
	// 对运维预置角色，测试管理员必须能 SET ROLE；不读取或修改密码。
	// 使用同一连接执行 SET ROLE 与 DDL 尝试。
	conn, err := p.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SET ROLE "+quoteIdentifier(runtimeRole)); err != nil {
		t.Fatalf("无法切换到 runtime 角色: %v", err)
	}
	expectCode(t, conn, ctx, "42501", "CREATE TABLE urbino.runtime_must_not_ddl(n integer)")
	_, _ = conn.Exec(ctx, "RESET ROLE")
}

func TestPostgresFreshRepeatAndConcurrentMigrate(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- postgres.Migrate(ctx, e.dbDSN) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("并行首次迁移失败: %v", err)
		}
	}
	if err := postgres.Migrate(ctx, e.dbDSN); err != nil {
		t.Fatalf("重复迁移失败: %v", err)
	}
	p, err := postgres.Open(ctx, e.dbDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := postgres.CheckSchema(ctx, p); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresTenantScopeAndUniqueness(t *testing.T) {
	e := newEnv(t)
	p := e.pool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	t1, t2, p1, p2, u1, u2 := testUUID(t), testUUID(t), testUUID(t), testUUID(t), testUUID(t), testUUID(t)
	for _, a := range [][2]string{{t1, "A"}, {t2, "B"}} {
		if _, err := p.Exec(ctx, "INSERT INTO urbino.tenants(id,name,status,currency,policy_version) VALUES ($1,$2,'active','USD','v1')", a[0], a[1]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.Exec(ctx, "INSERT INTO urbino.projects(tenant_id,id,name,status) VALUES ($1,$2,'p1','active'),($3,$4,'p2','active')", t1, p1, t2, p2); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "INSERT INTO urbino.users(tenant_id,id,display_name,status) VALUES ($1,$2,'u1','active'),($3,$4,'u2','active')", t1, u1, t2, u2); err != nil {
		t.Fatal(err)
	}
	expectCode(t, p, ctx, "23503", "INSERT INTO urbino.project_members(tenant_id,project_id,user_id,role) VALUES ($1,$2,$3,'member')", t1, p1, u2)
	if _, err := p.Exec(ctx, "INSERT INTO urbino.project_members(tenant_id,project_id,user_id,role) VALUES ($1,$2,$3,'member')", t1, p1, u1); err != nil {
		t.Fatal(err)
	}
	rid := testUUID(t)
	request := "INSERT INTO urbino.requests(id,tenant_id,project_id,request_id,body_digest,model,protocol,status,dispatch_status,usage_status,started_at) VALUES ($1,$2,$3,$4,decode('aa','hex'),'m','x','started','pending','unknown',now())"
	if _, err := p.Exec(ctx, request, rid, t1, p1, "r"); err != nil {
		t.Fatal(err)
	}
	expectCode(t, p, ctx, "23505", request, testUUID(t), t1, p1, "r")
	expectCode(t, p, ctx, "23503", request, testUUID(t), t1, p2, "cross-project")
	if _, err := p.Exec(ctx, request, testUUID(t), t2, p2, "r"); err != nil {
		t.Fatal(err)
	}
	aid := testUUID(t)
	attempt := "INSERT INTO urbino.request_attempts(id,request_id,tenant_id,attempt_no,dispatch_phase,result_class,status) VALUES ($1,$2,$3,$4,'preflight','none','started')"
	if _, err := p.Exec(ctx, attempt, aid, rid, t1, 1); err != nil {
		t.Fatal(err)
	}
	expectCode(t, p, ctx, "23505", attempt, testUUID(t), rid, t1, 1)
	expectCode(t, p, ctx, "23503", attempt, testUUID(t), rid, t2, 2)
	ue, pv := testUUID(t), testUUID(t)
	usage := "INSERT INTO urbino.usage_events(id,tenant_id,request_id,attempt_id,source,event_key,completeness) VALUES ($1,$2,$3,$4,'provider',$5,'known')"
	if _, err := p.Exec(ctx, usage, ue, t1, rid, aid, "e"); err != nil {
		t.Fatal(err)
	}
	expectCode(t, p, ctx, "23505", usage, testUUID(t), t1, rid, aid, "e")
	expectCode(t, p, ctx, "23503", usage, testUUID(t), t2, rid, aid, "cross-tenant")
	if _, err := p.Exec(ctx, "INSERT INTO urbino.price_versions(id,version,currency,effective_at,published_at) VALUES ($1,$2,'USD',now(),now())", pv, time.Now().UnixNano()); err != nil {
		t.Fatal(err)
	}
	settlement := "INSERT INTO urbino.settlements(tenant_id,request_id,price_version_id,price_snapshot,usage_event_id,amount_micros,status,created_at) VALUES ($1,$2,$3,'{}',$4,1,'posted',now())"
	if _, err := p.Exec(ctx, settlement, t1, rid, pv, ue); err != nil {
		t.Fatal(err)
	}
	expectCode(t, p, ctx, "23505", settlement, t1, rid, pv, ue)
	// 独立的未结算 request/usage 保证本断言只能命中跨租户 FK，而非 settlement 主键。
	rid2, aid2, ue2 := testUUID(t), testUUID(t), testUUID(t)
	if _, err := p.Exec(ctx, request, rid2, t1, p1, "unsettled"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, attempt, aid2, rid2, t1, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, usage, ue2, t1, rid2, aid2, "unsettled"); err != nil {
		t.Fatal(err)
	}
	expectCode(t, p, ctx, "23503", settlement, t2, rid2, pv, ue2)
}

func TestPostgresLedgerAndRuntimeRoleGuards(t *testing.T) {
	e := newEnv(t)
	p := e.pool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var schemaCreate, journalInsert, journalUpdate, journalDelete bool
	if err := p.QueryRow(ctx, `SELECT has_schema_privilege($1,'urbino','CREATE'),has_table_privilege($1,'urbino.journal_entries','INSERT'),has_table_privilege($1,'urbino.journal_entries','UPDATE'),has_table_privilege($1,'urbino.journal_entries','DELETE')`, runtimeRole).Scan(&schemaCreate, &journalInsert, &journalUpdate, &journalDelete); err != nil {
		t.Fatal(err)
	}
	if schemaCreate || journalInsert || journalUpdate || journalDelete {
		t.Fatalf("runtime 权限过宽: schema=%v insert=%v update=%v delete=%v", schemaCreate, journalInsert, journalUpdate, journalDelete)
	}
	assertRuntimeCannotDDL(t, e, p, ctx)
	tenant, txid := testUUID(t), testUUID(t)
	if _, err := p.Exec(ctx, "INSERT INTO urbino.tenants(id,name,status,currency,policy_version) VALUES ($1,'ledger','active','USD','v1')", tenant); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "INSERT INTO urbino.journal_transactions(id,tenant_id,business_key,currency,created_at) VALUES ($1,$2,$3,'USD',now())", txid, tenant, "bk-"+randomHex(t, 8)); err != nil {
		t.Fatal(err)
	}
	e1, e2 := testUUID(t), testUUID(t)
	if _, err := p.Exec(ctx, "INSERT INTO urbino.journal_entries(id,tenant_id,transaction_id,account_ref,amount_micros,currency) VALUES ($1,$2,$3,'a',-1,'USD'),($4,$2,$3,'b',1,'USD')", e1, tenant, txid, e2); err != nil {
		t.Fatal(err)
	}
	expectCode(t, p, ctx, "P0001", "UPDATE urbino.journal_entries SET amount_micros=2 WHERE id=$1", e1)
	expectCode(t, p, ctx, "P0001", "DELETE FROM urbino.journal_entries WHERE id=$1", e1)
	expectCode(t, p, ctx, "P0001", "UPDATE urbino.journal_transactions SET business_key='changed' WHERE id=$1", txid)
	expectCode(t, p, ctx, "P0001", "DELETE FROM urbino.journal_transactions WHERE id=$1", txid)
}

func TestPostgresRunTxCancellationRollback(t *testing.T) {
	e := newEnv(t)
	p := e.pool(t)
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
	callbackCtx, cancelCallback := context.WithCancel(ctx)
	err = postgres.RunTx(callbackCtx, conn, pgx.TxOptions{}, 1, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO urbino_tx_probe(n) VALUES (1)"); err != nil {
			return err
		}
		cancelCallback()
		return callbackCtx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	var n int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM urbino_tx_probe").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("rollback left %d rows", n)
	}
}

func TestPostgresSchemaCompatibilityRejectsIncompatibleVersion(t *testing.T) {
	e := newEnv(t)
	p := e.pool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := postgres.CheckSchema(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "UPDATE urbino.schema_version SET version=999 WHERE singleton=true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = p.Exec(context.Background(), "UPDATE urbino.schema_version SET version=1 WHERE singleton=true")
	})
	if err := postgres.CheckSchema(ctx, p); err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("不兼容 schema 被接受: %v", err)
	}
}

func expectCode(t *testing.T, db interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, ctx context.Context, code, query string, args ...any) {
	t.Helper()
	_, err := db.Exec(ctx, query, args...)
	if err == nil {
		t.Fatalf("expected PostgreSQL error %s", code)
	}
	var pe *pgconn.PgError
	if !errors.As(err, &pe) {
		t.Fatalf("expected PostgreSQL error code %s, got %T", code, err)
	}
	if pe.Code != code {
		t.Fatalf("expected PostgreSQL error code %s, got %s", code, pe.Code)
	}
}
func randomHex(t *testing.T, n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		if t != nil {
			t.Fatal(err)
		}
		panic(err)
	}
	return hex.EncodeToString(b)
}
func testUUID(t *testing.T) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b[0:4]) + "-" + hex.EncodeToString(b[4:6]) + "-" + hex.EncodeToString(b[6:8]) + "-" + hex.EncodeToString(b[8:10]) + "-" + hex.EncodeToString(b[10:16])
}
func quoteIdentifier(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
func withDatabase(u *url.URL, name string) *url.URL {
	c := *u
	c.Path = "/" + name
	c.RawPath = ""
	return &c
}
