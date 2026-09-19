// Phase 02 persistence integration tests (P02-T01..T07).
//
// Environment gates (enforced by requireRealPG):
//   - `-short` skips every real-PostgreSQL test with a distinguishable
//     `NOT_RUN <TestName>: ...` marker.
//   - The only accepted database target is the explicit
//     URBINO_TEST_DATABASE_DSN, which must pass the local-test safety rules
//     in tests/integration/harness.go (loopback host, *_test database name).
//     There is no default DSN: without the variable the tests report NOT_RUN.
//
// Migrations are applied exclusively by the production `urbino migrate`
// command (DSN supplied through a 0600 file referenced by database.dsn_file,
// never by argv or process environment). No production code is modified by
// this suite and no secret is ever printed: DSNs and command output pass
// through integration.RedactDSN before they can reach a failure message.
package integration_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"example.com/urbino/internal/storage/postgres"
	"example.com/urbino/tests/integration"
)

// The application role credentials are synthetic values that exist only
// inside the disposable test database; they are re-created on every run.
const (
	appRoleName     = "urbino_p02_app"
	appRolePassword = "synthetic-p02-app-role-only"
)

// p02Env is the lazily created, package-wide test environment.
type p02Env struct {
	repoRoot   string
	bin        string
	tempDir    string
	dsn        integration.TestDSN
	admin      *pgx.Conn
	appDSN     string
	migrations []string
}

var (
	envOnce sync.Once
	env     *p02Env
	envErr  error
)

// requireRealPG enforces the phase 02 gates. Skip reasons always start with
// the NOT_RUN marker so gates can distinguish "did not run" from "passed".
// A DSN that is configured but broken (unreachable server, unbuildable
// binary, failed migration) is a hard test failure, never a silent skip.
func requireRealPG(t *testing.T) *p02Env {
	t.Helper()
	if testing.Short() {
		t.Skipf("NOT_RUN %s: -short mode skips real PostgreSQL integration tests", t.Name())
	}
	dsn, reason, ok := integration.ResolveTestDSN(os.Getenv)
	if !ok {
		t.Skipf("NOT_RUN %s: %s", t.Name(), reason)
	}
	envOnce.Do(func() { env, envErr = setupEnv(dsn) })
	if envErr != nil {
		t.Fatalf("%s: test environment setup failed: %v", t.Name(), envErr)
	}
	return env
}

func setupEnv(dsn integration.TestDSN) (*p02Env, error) {
	repoRoot, err := integration.FindRepoRoot(".")
	if err != nil {
		return nil, err
	}
	migrations, err := integration.ListMigrationFiles(repoRoot)
	if err != nil {
		return nil, err
	}
	tempDir, err := os.MkdirTemp("", "urbino-p02-tests-")
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*p02Env, error) {
		os.RemoveAll(tempDir)
		return nil, err
	}
	bin := filepath.Join(tempDir, "urbino")
	build := exec.Command("go", "build", "-o", bin, "./cmd/urbino")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		return fail(fmt.Errorf("build ./cmd/urbino: %w: %s", err, integration.RedactDSN(string(out))))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, dsn.Raw)
	if err != nil {
		return fail(fmt.Errorf("connect URBINO_TEST_DATABASE_DSN: %s", integration.RedactDSN(err.Error())))
	}
	e := &p02Env{repoRoot: repoRoot, bin: bin, tempDir: tempDir, dsn: dsn, admin: admin, migrations: migrations}
	if err := e.resetSchema(ctx); err != nil {
		admin.Close(context.Background())
		return fail(err)
	}
	if exit, out, err := e.migrateDSN(ctx, dsn.Raw); err != nil || exit != 0 {
		admin.Close(context.Background())
		return fail(fmt.Errorf("initial `urbino migrate` exited %d: %s", exit, out))
	}
	appDSN, err := dsn.WithCredentials(appRoleName, appRolePassword)
	if err != nil {
		admin.Close(context.Background())
		return fail(err)
	}
	if err := e.provisionAppRole(ctx); err != nil {
		admin.Close(context.Background())
		return fail(err)
	}
	e.appDSN = appDSN
	return e, nil
}

// resetSchema gives every full suite run a clean public schema in the
// *_test database. The safety gate guarantees the database is a disposable
// local test database before this ever runs.
func (e *p02Env) resetSchema(ctx context.Context) error {
	_, err := e.admin.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
	if err != nil {
		return fmt.Errorf("reset public schema: %s", integration.RedactDSN(err.Error()))
	}
	return nil
}

// provisionAppRole materializes the phase 02 role separation contract: the
// application role can read domain state and mutate request/business intake
// rows only. It cannot mutate balances, pricing, tenancy metadata, immutable
// journal rows, migration history, or create schema objects.
func (e *p02Env) provisionAppRole(ctx context.Context) error {
	database := quoteIdent(e.dsn.Database)
	sqlStmt := fmt.Sprintf(`
    DROP ROLE IF EXISTS %s;
    CREATE ROLE %s LOGIN PASSWORD '%s';
    REVOKE CREATE ON SCHEMA public FROM PUBLIC;
    REVOKE TEMP ON DATABASE %s FROM PUBLIC;
    REVOKE TEMP ON DATABASE %s FROM %s;
    GRANT USAGE ON SCHEMA public TO %s;
    REVOKE ALL ON ALL TABLES IN SCHEMA public FROM %s;
    GRANT SELECT ON ALL TABLES IN SCHEMA public TO %s;
    GRANT INSERT, UPDATE ON requests, request_attempts TO %s;`,
		quoteIdent(appRoleName), quoteIdent(appRoleName), appRolePassword, database, database,
		quoteIdent(appRoleName), quoteIdent(appRoleName), quoteIdent(appRoleName), quoteIdent(appRoleName), quoteIdent(appRoleName))
	if _, err := e.admin.Exec(ctx, sqlStmt); err != nil {
		return fmt.Errorf("provision %s role: %s", appRoleName, integration.RedactDSN(err.Error()))
	}
	return nil
}

var migrateFileCounter atomic.Int64

// migrateDSN runs the production migrate command against rawDSN. The DSN
// reaches the binary only through a 0600 file referenced by the config's
// database.dsn_file; it never appears in argv or the child environment.
// Output is redacted before it is returned.
func (e *p02Env) migrateDSN(ctx context.Context, rawDSN string) (int, string, error) {
	if _, hasCancel := ctx.Deadline(); !hasCancel {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
	}
	seq := migrateFileCounter.Add(1)
	dsnFile := filepath.Join(e.tempDir, fmt.Sprintf("migrate-%d.dsn", seq))
	if err := os.WriteFile(dsnFile, []byte(rawDSN+"\n"), 0o600); err != nil {
		return -1, "", fmt.Errorf("write restricted DSN file: %w", err)
	}
	cfgFile := filepath.Join(e.tempDir, fmt.Sprintf("migrate-%d.yaml", seq))
	cfg := fmt.Sprintf("environment: test\nhealth_addr: 127.0.0.1:0\nlog_level: warn\ndatabase:\n  dsn_file: %s\n", dsnFile)
	if err := os.WriteFile(cfgFile, []byte(cfg), 0o600); err != nil {
		return -1, "", fmt.Errorf("write migrate config: %w", err)
	}
	cmd := exec.CommandContext(ctx, e.bin, "--config", cfgFile, "migrate")
	cmd.Dir = e.tempDir
	cmd.Env = cleanEnv()
	out, err := cmd.CombinedOutput()
	exit := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exit = exitErr.ExitCode()
		} else {
			return -1, integration.RedactDSN(string(out)), fmt.Errorf("run urbino migrate: %w", err)
		}
	}
	return exit, integration.RedactDSN(string(out)), nil
}

// serveDSN runs the production serve command with a restricted DSN file.
func (e *p02Env) serveDSN(ctx context.Context, rawDSN string) (int, string, error) {
	seq := migrateFileCounter.Add(1)
	dsnFile := filepath.Join(e.tempDir, fmt.Sprintf("serve-%d.dsn", seq))
	if err := os.WriteFile(dsnFile, []byte(rawDSN+"\n"), 0o600); err != nil {
		return -1, "", fmt.Errorf("write restricted DSN file: %w", err)
	}
	cfgFile := filepath.Join(e.tempDir, fmt.Sprintf("serve-%d.yaml", seq))
	cfg := fmt.Sprintf("environment: test\nhealth_addr: 127.0.0.1:0\nlog_level: warn\ndatabase:\n  dsn_file: %s\n", dsnFile)
	if err := os.WriteFile(cfgFile, []byte(cfg), 0o600); err != nil {
		return -1, "", fmt.Errorf("write serve config: %w", err)
	}
	cmd := exec.CommandContext(ctx, e.bin, "--config", cfgFile, "serve")
	cmd.Dir = e.tempDir
	cmd.Env = cleanEnv()
	out, err := cmd.CombinedOutput()
	exit := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exit = exitErr.ExitCode()
		} else {
			return -1, integration.RedactDSN(string(out)), fmt.Errorf("run urbino serve: %w", err)
		}
	}
	return exit, integration.RedactDSN(string(out)), nil
}

// scratchDatabase creates a fresh disposable database (name keeps the
// mandatory _test suffix) and registers cleanup that drops it again.
func (e *p02Env) scratchDatabase(t *testing.T, suffix string) (name string, rawDSN string, conn *pgx.Conn) {
	t.Helper()
	ctx := t.Context()
	name = e.dsn.DatabaseNamed(suffix)
	drop := fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", quoteIdent(name))
	if _, err := e.admin.Exec(ctx, drop); err != nil {
		t.Fatalf("drop scratch database %s: %s", name, integration.RedactDSN(err.Error()))
	}
	if _, err := e.admin.Exec(ctx, "CREATE DATABASE "+quoteIdent(name)); err != nil {
		t.Fatalf("create scratch database %s: %s", name, integration.RedactDSN(err.Error()))
	}
	raw, err := e.dsn.WithDatabase(name)
	if err != nil {
		t.Fatalf("derive scratch DSN: %v", err)
	}
	conn, err = pgx.Connect(ctx, raw)
	if err != nil {
		t.Fatalf("connect scratch database %s: %s", name, integration.RedactDSN(err.Error()))
	}
	t.Cleanup(func() {
		conn.Close(context.Background())
		_, _ = e.admin.Exec(context.Background(), drop)
	})
	return name, raw, conn
}

// cleanEnv removes every URBINO_* variable so subprocess runs depend only on
// the explicit --config file, mirroring the phase 00 foundation harness.
func cleanEnv() []string {
	var envOut []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "URBINO_") {
			continue
		}
		envOut = append(envOut, kv)
	}
	return envOut
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func TestMain(m *testing.M) {
	code := m.Run()
	if env != nil {
		if env.admin != nil {
			env.admin.Close(context.Background())
		}
		os.RemoveAll(env.tempDir)
	}
	os.Exit(code)
}

var testSequence atomic.Int64

func testUUID() string {
	n := testSequence.Add(1)
	return fmt.Sprintf("00000000-0000-7000-8000-%012x", n)
}

func insertTenantAndProject(t *testing.T, conn *pgx.Conn) (string, string) {
	t.Helper()
	tenant, project := testUUID(), testUUID()
	if _, err := conn.Exec(t.Context(), `INSERT INTO tenants(id,name,status,currency,policy_version) VALUES ($1,$2,'active','USD',1)`, tenant, "phase02-tenant"); err != nil {
		t.Fatalf("insert tenant: %s", integration.RedactDSN(err.Error()))
	}
	if _, err := conn.Exec(t.Context(), `INSERT INTO projects(tenant_id,id,name,status) VALUES ($1,$2,'phase02-project','active')`, tenant, project); err != nil {
		t.Fatalf("insert project: %s", integration.RedactDSN(err.Error()))
	}
	return tenant, project
}

func insertRequest(t *testing.T, conn *pgx.Conn, tenant, project, idempotency string) string {
	t.Helper()
	request := testUUID()
	_, err := conn.Exec(t.Context(), `INSERT INTO requests(tenant_id,id,project_id,idempotency_digest,body_digest,model,protocol,status,dispatch_status,usage_status,started_at) VALUES ($1,$2,$3,$4,decode('01','hex'),'phase02-model','json','created','pending','missing',now())`, tenant, request, project, []byte(idempotency))
	if err != nil {
		t.Fatalf("insert request: %s", integration.RedactDSN(err.Error()))
	}
	return request
}

func TestP02T01CompositeTenantForeignKeys(t *testing.T) {
	e := requireRealPG(t)
	tenant, project := insertTenantAndProject(t, e.admin)
	user, price := testUUID(), testUUID()
	if _, err := e.admin.Exec(t.Context(), `INSERT INTO users(tenant_id,id,display_name,status) VALUES ($1,$2,'phase02-user','active')`, tenant, user); err != nil {
		t.Fatalf("insert user: %s", integration.RedactDSN(err.Error()))
	}
	if _, err := e.admin.Exec(t.Context(), `INSERT INTO price_versions(tenant_id,id,currency,effective_at,status) VALUES ($1,$2,'USD',now(),'draft')`, tenant, price); err != nil {
		t.Fatalf("insert price version: %s", integration.RedactDSN(err.Error()))
	}
	otherTenant := testUUID()
	if _, err := e.admin.Exec(t.Context(), `INSERT INTO tenants(id,name,status,currency,policy_version) VALUES ($1,'phase02-other','active','USD',1)`, otherTenant); err != nil {
		t.Fatalf("insert second tenant: %s", integration.RedactDSN(err.Error()))
	}
	reject := func(name, statement string, args ...any) {
		t.Helper()
		if _, err := e.admin.Exec(t.Context(), statement, args...); err == nil {
			t.Fatalf("cross-tenant %s reference accepted", name)
		}
	}
	reject("project", `INSERT INTO requests(tenant_id,id,project_id,body_digest,model,protocol,status,dispatch_status,usage_status,started_at) VALUES ($1,$2,$3,decode('02','hex'),'m','json','created','pending','missing',now())`, otherTenant, testUUID(), project)
	reject("user", `INSERT INTO requests(tenant_id,id,project_id,user_id,body_digest,model,protocol,status,dispatch_status,usage_status,started_at) VALUES ($1,$2,$3,$4,decode('03','hex'),'m','json','created','pending','missing',now())`, tenant, testUUID(), project, testUUID())
	reject("price version", `INSERT INTO requests(tenant_id,id,project_id,price_version_id,body_digest,model,protocol,status,dispatch_status,usage_status,started_at) VALUES ($1,$2,$3,$4,decode('04','hex'),'m','json','created','pending','missing',now())`, otherTenant, testUUID(), project, price)
	request := insertRequest(t, e.admin, tenant, project, "foreign-key-request")
	reject("request attempt", `INSERT INTO request_attempts(tenant_id,id,request_id,attempt_no,credential_version,egress_version,dispatch_phase) VALUES ($1,$2,$3,1,1,1,'started')`, otherTenant, testUUID(), request)
	reject("settlement request", `INSERT INTO settlements(tenant_id,id,request_id,business_key,price_version_id,currency,price_snapshot,amount_micros,status) VALUES ($1,$2,$3,'foreign-settlement',$4,'USD','{}'::jsonb,1,'posted')`, otherTenant, testUUID(), request, price)
	transaction := testUUID()
	tx, err := e.admin.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin journal fixture: %s", integration.RedactDSN(err.Error()))
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO journal_transactions(tenant_id,id,business_key,currency) VALUES ($1,$2,'foreign-journal','USD')`, tenant, transaction); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatalf("insert journal transaction: %s", integration.RedactDSN(err.Error()))
	}
	for _, entry := range []struct {
		id, account string
		amount      int
	}{{testUUID(), "cash", 1}, {testUUID(), "revenue", -1}} {
		if _, err := tx.Exec(t.Context(), `INSERT INTO journal_entries(tenant_id,id,transaction_id,currency,account_ref,amount_micros) VALUES ($1,$2,$3,'USD',$4,$5)`, tenant, entry.id, transaction, entry.account, entry.amount); err != nil {
			_ = tx.Rollback(t.Context())
			t.Fatalf("insert journal entry: %s", integration.RedactDSN(err.Error()))
		}
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("commit journal fixture: %s", integration.RedactDSN(err.Error()))
	}
	reject("journal entry", `INSERT INTO journal_entries(tenant_id,id,transaction_id,currency,account_ref,amount_micros) VALUES ($1,$2,$3,'USD','cash',1)`, otherTenant, testUUID(), transaction)
}

func TestP02T02BusinessKeyUniqueness(t *testing.T) {
	e := requireRealPG(t)
	tenant, project := insertTenantAndProject(t, e.admin)
	request := insertRequest(t, e.admin, tenant, project, "idempotency")
	_, err := e.admin.Exec(t.Context(), `INSERT INTO requests(tenant_id,id,project_id,idempotency_digest,body_digest,model,protocol,status,dispatch_status,usage_status,started_at) VALUES ($1,$2,$3,$4,decode('03','hex'),'m','json','created','pending','missing',now())`, tenant, testUUID(), project, []byte("idempotency"))
	if err == nil {
		t.Fatal("duplicate request idempotency key accepted")
	}
	attempt := testUUID()
	if _, err := e.admin.Exec(t.Context(), `INSERT INTO request_attempts(tenant_id,id,request_id,attempt_no,credential_version,egress_version,dispatch_phase) VALUES ($1,$2,$3,1,1,1,'started')`, tenant, attempt, request); err != nil {
		t.Fatalf("insert attempt: %s", integration.RedactDSN(err.Error()))
	}
	_, err = e.admin.Exec(t.Context(), `INSERT INTO request_attempts(tenant_id,id,request_id,attempt_no,credential_version,egress_version,dispatch_phase) VALUES ($1,$2,$3,1,1,1,'started')`, tenant, testUUID(), request)
	if err == nil {
		t.Fatal("duplicate attempt number accepted")
	}
	emptyPrice := testUUID()
	if _, err := e.admin.Exec(t.Context(), `INSERT INTO price_versions(tenant_id,id,currency,effective_at,status) VALUES ($1,$2,'USD',now(),'draft')`, tenant, emptyPrice); err != nil {
		t.Fatalf("insert empty price version: %s", integration.RedactDSN(err.Error()))
	}
	if _, err := e.admin.Exec(t.Context(), `UPDATE price_versions SET status='published' WHERE tenant_id=$1 AND id=$2`, tenant, emptyPrice); err == nil {
		t.Fatal("published price version without price items accepted")
	}
	price := testUUID()
	if _, err := e.admin.Exec(t.Context(), `INSERT INTO price_versions(tenant_id,id,currency,effective_at,status) VALUES ($1,$2,'USD',now(),'draft')`, tenant, price); err != nil {
		t.Fatalf("insert price version: %s", integration.RedactDSN(err.Error()))
	}
	if _, err := e.admin.Exec(t.Context(), `INSERT INTO price_items(tenant_id,id,price_version_id,model,tier,dimension,unit_price) VALUES ($1,$2,$3,'phase02-model','standard','input',2.000000000000)`, tenant, testUUID(), price); err != nil {
		t.Fatalf("insert price item: %s", integration.RedactDSN(err.Error()))
	}
	if _, err := e.admin.Exec(t.Context(), `UPDATE price_versions SET status='published' WHERE tenant_id=$1 AND id=$2`, tenant, price); err != nil {
		t.Fatalf("publish price version: %s", integration.RedactDSN(err.Error()))
	}
	atomicPrice := testUUID()
	atomicTx, err := e.admin.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin atomic price fixture: %s", integration.RedactDSN(err.Error()))
	}
	if _, err := atomicTx.Exec(t.Context(), `INSERT INTO price_versions(tenant_id,id,currency,effective_at,status) VALUES ($1,$2,'USD',now(),'draft')`, tenant, atomicPrice); err != nil {
		_ = atomicTx.Rollback(t.Context())
		t.Fatalf("insert atomic price version: %s", integration.RedactDSN(err.Error()))
	}
	if _, err := atomicTx.Exec(t.Context(), `INSERT INTO price_items(tenant_id,id,price_version_id,model,tier,dimension,unit_price) VALUES ($1,$2,$3,'phase02-atomic','standard','input',3.000000000000)`, tenant, testUUID(), atomicPrice); err != nil {
		_ = atomicTx.Rollback(t.Context())
		t.Fatalf("insert atomic price item: %s", integration.RedactDSN(err.Error()))
	}
	if _, err := atomicTx.Exec(t.Context(), `UPDATE price_versions SET status='published' WHERE tenant_id=$1 AND id=$2`, tenant, atomicPrice); err != nil {
		_ = atomicTx.Rollback(t.Context())
		t.Fatalf("publish atomic price version: %s", integration.RedactDSN(err.Error()))
	}
	if err := atomicTx.Commit(t.Context()); err != nil {
		t.Fatalf("commit atomic price fixture: %s", integration.RedactDSN(err.Error()))
	}
	if _, err := e.admin.Exec(t.Context(), `INSERT INTO settlements(tenant_id,id,request_id,business_key,price_version_id,currency,price_snapshot,amount_micros,status) VALUES ($1,$2,$3,'settlement-key',$4,'USD','{"model":"phase02-model","dimension":"input","unit_price":"2.000000000000"}'::jsonb,1,'posted')`, tenant, testUUID(), request, price); err != nil {
		t.Fatalf("insert settlement: %s", integration.RedactDSN(err.Error()))
	}
	_, err = e.admin.Exec(t.Context(), `INSERT INTO settlements(tenant_id,id,request_id,business_key,price_version_id,currency,price_snapshot,amount_micros,status) VALUES ($1,$2,$3,'settlement-key',$4,'USD','{"model":"phase02-model","dimension":"input","unit_price":"2.000000000000"}'::jsonb,1,'posted')`, tenant, testUUID(), request, price)
	if err == nil {
		t.Fatal("duplicate settlement business key accepted")
	}
}

func TestP02T03ConcurrentMigratorLock(t *testing.T) {
	e := requireRealPG(t)
	_, rawDSN, conn := e.scratchDatabase(t, "_parallel")
	var wg sync.WaitGroup
	results := make(chan struct {
		exit int
		out  string
		err  error
	}, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			exit, out, err := e.migrateDSN(t.Context(), rawDSN)
			results <- struct {
				exit int
				out  string
				err  error
			}{exit, out, err}
		}()
	}
	wg.Wait()
	close(results)
	for result := range results {
		if result.err != nil || result.exit != 0 {
			t.Fatalf("concurrent migrate failed exit=%d err=%v out=%q", result.exit, result.err, result.out)
		}
	}
	var count int
	if err := conn.QueryRow(t.Context(), `SELECT count(*) FROM urbino_schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count migration history: %s", integration.RedactDSN(err.Error()))
	}
	if count != len(e.migrations) {
		t.Fatalf("migration history rows = %d, want %d", count, len(e.migrations))
	}
}

func TestP02T04ApplicationRoleCannotDDLOrMutateLedger(t *testing.T) {
	e := requireRealPG(t)
	tenant := testUUID()
	if _, err := e.admin.Exec(t.Context(), `INSERT INTO tenants(id,name,status,currency,policy_version) VALUES ($1,'phase02-role','active','USD',1)`, tenant); err != nil {
		t.Fatalf("insert role test tenant: %s", integration.RedactDSN(err.Error()))
	}
	project := testUUID()
	if _, err := e.admin.Exec(t.Context(), `INSERT INTO projects(tenant_id,id,name,status) VALUES ($1,$2,'phase02-role-project','active')`, tenant, project); err != nil {
		t.Fatalf("insert role test project: %s", integration.RedactDSN(err.Error()))
	}
	user := testUUID()
	if _, err := e.admin.Exec(t.Context(), `INSERT INTO users(tenant_id,id,display_name,status) VALUES ($1,$2,'phase02-role-user','active')`, tenant, user); err != nil {
		t.Fatalf("insert role test user: %s", integration.RedactDSN(err.Error()))
	}
	unbalanced := testUUID()
	if _, err := e.admin.Exec(t.Context(), `INSERT INTO journal_transactions(tenant_id,id,business_key,currency) VALUES ($1,$2,'unbalanced-business-key','USD')`, tenant, unbalanced); err == nil {
		t.Fatal("unbalanced journal transaction accepted")
	}
	transaction := testUUID()
	tx, err := e.admin.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin journal fixture: %s", integration.RedactDSN(err.Error()))
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO journal_transactions(tenant_id,id,business_key,currency) VALUES ($1,$2,'role-business-key','USD')`, tenant, transaction); err != nil {
		t.Fatalf("insert journal transaction: %s", integration.RedactDSN(err.Error()))
	}
	for _, entry := range []struct {
		id      string
		account string
		amount  int
	}{{testUUID(), "cash", 1}, {testUUID(), "revenue", -1}} {
		if _, err := tx.Exec(t.Context(), `INSERT INTO journal_entries(tenant_id,id,transaction_id,currency,account_ref,amount_micros) VALUES ($1,$2,$3,'USD',$4,$5)`, tenant, entry.id, transaction, entry.account, entry.amount); err != nil {
			_ = tx.Rollback(t.Context())
			t.Fatalf("insert journal entry: %s", integration.RedactDSN(err.Error()))
		}
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("commit balanced journal fixture: %s", integration.RedactDSN(err.Error()))
	}
	nonzeroTx := testUUID()
	nonzero, err := e.admin.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin nonzero journal fixture: %s", integration.RedactDSN(err.Error()))
	}
	if _, err := nonzero.Exec(t.Context(), `INSERT INTO journal_transactions(tenant_id,id,business_key,currency) VALUES ($1,$2,'nonzero-business-key','USD')`, tenant, nonzeroTx); err != nil {
		t.Fatalf("insert nonzero journal transaction: %s", integration.RedactDSN(err.Error()))
	}
	for _, amount := range []int{1, 1} {
		if _, err := nonzero.Exec(t.Context(), `INSERT INTO journal_entries(tenant_id,id,transaction_id,currency,account_ref,amount_micros) VALUES ($1,$2,$3,'USD','nonzero',$4)`, tenant, testUUID(), nonzeroTx, amount); err != nil {
			_ = nonzero.Rollback(t.Context())
			t.Fatalf("insert nonzero journal entry: %s", integration.RedactDSN(err.Error()))
		}
	}
	if err := nonzero.Commit(t.Context()); err == nil {
		t.Fatal("nonzero journal transaction accepted")
	}
	mixedTx := testUUID()
	mixed, err := e.admin.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin mixed-currency journal fixture: %s", integration.RedactDSN(err.Error()))
	}
	if _, err := mixed.Exec(t.Context(), `INSERT INTO journal_transactions(tenant_id,id,business_key,currency) VALUES ($1,$2,'mixed-business-key','USD')`, tenant, mixedTx); err != nil {
		_ = mixed.Rollback(t.Context())
		t.Fatalf("insert mixed journal transaction: %s", integration.RedactDSN(err.Error()))
	}
	if _, err := mixed.Exec(t.Context(), `INSERT INTO journal_entries(tenant_id,id,transaction_id,currency,account_ref,amount_micros) VALUES ($1,$2,$3,'EUR','mixed',1)`, tenant, testUUID(), mixedTx); err == nil {
		_ = mixed.Rollback(t.Context())
		t.Fatal("mixed-currency journal entry accepted")
	}
	_ = mixed.Rollback(t.Context())
	price := testUUID()
	priceItem := testUUID()
	if _, err := e.admin.Exec(t.Context(), `INSERT INTO price_versions(tenant_id,id,currency,effective_at,status) VALUES ($1,$2,'USD',now(),'draft')`, tenant, price); err != nil {
		t.Fatalf("insert role test price version: %s", integration.RedactDSN(err.Error()))
	}
	if _, err := e.admin.Exec(t.Context(), `INSERT INTO price_items(tenant_id,id,price_version_id,model,tier,dimension,unit_price) VALUES ($1,$2,$3,'role-model','standard','input',3.000000000000)`, tenant, priceItem, price); err != nil {
		t.Fatalf("insert role test price item: %s", integration.RedactDSN(err.Error()))
	}
	if _, err := e.admin.Exec(t.Context(), `UPDATE price_versions SET status='published' WHERE tenant_id=$1 AND id=$2`, tenant, price); err != nil {
		t.Fatalf("publish role test price: %s", integration.RedactDSN(err.Error()))
	}
	if _, err := e.admin.Exec(t.Context(), `UPDATE price_items SET unit_price=4.000000000000 WHERE tenant_id=$1 AND id=$2`, tenant, priceItem); err == nil {
		t.Fatal("published price item update accepted")
	}
	if _, err := e.admin.Exec(t.Context(), `DELETE FROM price_items WHERE tenant_id=$1 AND id=$2`, tenant, priceItem); err == nil {
		t.Fatal("published price item delete accepted")
	}
	if _, err := e.admin.Exec(t.Context(), `INSERT INTO price_items(tenant_id,id,price_version_id,model,tier,dimension,unit_price) VALUES ($1,$2,$3,'role-model-2','standard','output',4.000000000000)`, tenant, testUUID(), price); err == nil {
		t.Fatal("published price item insert accepted")
	}
	app, err := pgx.Connect(t.Context(), e.appDSN)
	if err != nil {
		t.Fatalf("connect app role: %s", integration.RedactDSN(err.Error()))
	}
	defer app.Close(context.Background())
	for name, statement := range map[string]string{
		"ddl":               `CREATE TABLE phase02_forbidden_table(id integer)`,
		"temp ddl":          `CREATE TEMP TABLE phase02_forbidden_temp(id integer)`,
		"tenant update":     `UPDATE tenants SET name='changed' WHERE id=$1 AND id<>$2`,
		"project delete":    `DELETE FROM projects WHERE tenant_id=$1 AND id=$2`,
		"user update":       `UPDATE users SET display_name='changed' WHERE tenant_id=$1 AND id=$2`,
		"price item update": `UPDATE price_items SET unit_price=5.000000000000 WHERE tenant_id=$1 AND price_version_id=$2`,
		"price item delete": `DELETE FROM price_items WHERE tenant_id=$1 AND price_version_id=$2`,
		"journal insert":    `INSERT INTO journal_entries(tenant_id,id,transaction_id,currency,account_ref,amount_micros) VALUES ($1,$2,$2,'USD','app',1)`,
		"journal update":    `UPDATE journal_entries SET account_ref='changed' WHERE tenant_id=$1 AND transaction_id=$2`,
		"journal delete":    `DELETE FROM journal_entries WHERE tenant_id=$1 AND transaction_id=$2`,
		"journal tx insert": `INSERT INTO journal_transactions(tenant_id,id,business_key,currency) VALUES ($1,$2,'app-insert','USD')`,
		"journal tx update": `UPDATE journal_transactions SET business_key='changed' WHERE tenant_id=$1 AND id=$2`,
		"balance update":    `UPDATE billing_accounts SET posted_balance_micros=posted_balance_micros+1 WHERE tenant_id=$1`,
		"settlement delete": `DELETE FROM settlements WHERE tenant_id=$1`,
		"history update":    `UPDATE urbino_schema_migrations SET name='tampered' WHERE version=1`,
		"history insert":    `INSERT INTO urbino_schema_migrations(version,name,checksum) VALUES (9999,'tampered',decode('00','hex'))`,
	} {
		args := []any{tenant, transaction}
		if name == "ddl" || name == "temp ddl" || name == "balance update" || name == "settlement delete" || name == "history update" || name == "history insert" {
			args = nil
		}
		if _, err := app.Exec(t.Context(), statement, args...); err == nil {
			t.Fatalf("application role accepted forbidden %s", name)
		}
	}
}

func TestP02T05CanceledTransactionRollsBack(t *testing.T) {
	e := requireRealPG(t)
	db, err := postgres.Open(t.Context(), postgres.Config{DSN: e.dsn.Raw, StatementTimeout: 2 * time.Second, LockTimeout: time.Second})
	if err != nil {
		t.Fatalf("open database: %s", postgres.SafeErrorMessage(err))
	}
	defer db.Close()
	tenant := testUUID()
	canceled, cancel := context.WithCancel(t.Context())
	err = db.WithTx(canceled, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO tenants(id,name,status,currency,policy_version) VALUES ($1,'rollback-tenant','active','USD',1)`, tenant); err != nil {
			return err
		}
		cancel()
		_, err := tx.Exec(ctx, `SELECT pg_sleep(1)`)
		return err
	})
	if err == nil {
		t.Fatal("canceled in-flight transaction succeeded")
	}
	var count int
	if err := e.admin.QueryRow(t.Context(), `SELECT count(*) FROM tenants WHERE id=$1`, tenant).Scan(&count); err != nil {
		t.Fatalf("check rollback: %s", integration.RedactDSN(err.Error()))
	}
	if count != 0 {
		t.Fatal("canceled transaction left committed data")
	}
	if err := db.Ping(t.Context()); err != nil {
		t.Fatalf("pool connection unusable after rollback: %s", postgres.SafeErrorMessage(err))
	}
}
func schemaSnapshot(t *testing.T, conn *pgx.Conn) string {
	t.Helper()
	rows, err := conn.Query(t.Context(), integration.SchemaSnapshotSQL)
	if err != nil {
		t.Fatalf("schema snapshot query: %s", integration.RedactDSN(err.Error()))
	}
	defer rows.Close()
	var snapshot strings.Builder
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			t.Fatalf("schema snapshot row: %s", integration.RedactDSN(err.Error()))
		}
		fmt.Fprintf(&snapshot, "%v\n", values)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("schema snapshot rows: %s", integration.RedactDSN(err.Error()))
	}
	return snapshot.String()
}

func TestP02T06SchemaCompatibilityRejectsNewerVersion(t *testing.T) {
	e := requireRealPG(t)
	_, rawDSN, conn := e.scratchDatabase(t, "_compat")
	if exit, out, err := e.migrateDSN(t.Context(), rawDSN); err != nil || exit != 0 {
		t.Fatalf("initial scratch migrate exit=%d err=%v out=%q", exit, err, out)
	}
	before := schemaSnapshot(t, conn)
	if _, err := conn.Exec(t.Context(), `INSERT INTO urbino_schema_migrations(version,name,checksum) VALUES (999,'future',decode(repeat('00',32),'hex'))`); err != nil {
		t.Fatalf("insert future schema marker: %s", integration.RedactDSN(err.Error()))
	}
	if exit, out, err := e.migrateDSN(t.Context(), rawDSN); err != nil || exit == 0 || !strings.Contains(out, "schema compatibility check failed") {
		t.Fatalf("newer schema migrate exit=%d err=%v out=%q", exit, err, out)
	}
	serveCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if exit, out, err := e.serveDSN(serveCtx, rawDSN); err != nil || exit == 0 || !strings.Contains(out, "schema compatibility check failed") {
		t.Fatalf("newer schema serve exit=%d err=%v out=%q", exit, err, out)
	}
	after := schemaSnapshot(t, conn)
	if before != after {
		t.Fatal("incompatible migration changed schema")
	}
}

func TestP02T06MigrationHistoryDriftRejected(t *testing.T) {
	e := requireRealPG(t)
	_, rawDSN, conn := e.scratchDatabase(t, "_drift")
	if exit, out, err := e.migrateDSN(t.Context(), rawDSN); err != nil || exit != 0 {
		t.Fatalf("initial drift migrate exit=%d err=%v out=%q", exit, err, out)
	}
	before := schemaSnapshot(t, conn)
	if _, err := conn.Exec(t.Context(), `UPDATE urbino_schema_migrations SET checksum=decode(repeat('ff',32),'hex') WHERE version=1`); err != nil {
		t.Fatalf("tamper migration checksum: %s", integration.RedactDSN(err.Error()))
	}
	if exit, out, err := e.migrateDSN(t.Context(), rawDSN); err != nil || exit == 0 || !strings.Contains(out, "schema migration history drift detected") {
		t.Fatalf("drift migrate exit=%d err=%v out=%q", exit, err, out)
	}
	serveCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if exit, out, err := e.serveDSN(serveCtx, rawDSN); err != nil || exit == 0 || !strings.Contains(out, "schema migration history drift detected") {
		t.Fatalf("drift schema serve exit=%d err=%v out=%q", exit, err, out)
	}
	if after := schemaSnapshot(t, conn); before != after {
		t.Fatal("migration history drift attempt changed schema")
	}
}

func TestP02T07DatabaseErrorsAreRedacted(t *testing.T) {
	e := requireRealPG(t)
	secret := "postgres://app:phase02-secret@127.0.0.1:1/urbino_test"
	db, err := postgres.Open(t.Context(), postgres.Config{DSN: secret, ConnectTimeout: 500 * time.Millisecond})
	if err != nil {
		t.Fatalf("Open returned setup error: %s", postgres.SafeErrorMessage(err))
	}
	pingCtx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	err = db.Ping(pingCtx)
	cancel()
	db.Close()
	if err == nil {
		t.Fatal("unreachable database unexpectedly responded")
	}
	message := postgres.SafeErrorMessage(err)
	if message == "" || strings.Contains(message, "phase02-secret") || strings.Contains(message, secret) {
		t.Fatalf("real database error leaked sensitive detail: %q", message)
	}
	exit, out, runErr := e.migrateDSN(t.Context(), secret)
	if runErr != nil || exit == 0 || strings.Contains(out, "phase02-secret") || strings.Contains(out, secret) {
		t.Fatalf("migrate error redaction failed exit=%d err=%v out=%q", exit, runErr, out)
	}
}
