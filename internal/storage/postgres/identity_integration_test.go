package postgres_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"example.com/urbino/internal/auth"
	"example.com/urbino/internal/domain"
	"example.com/urbino/internal/storage/migrate"
	"example.com/urbino/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgtype"
)

const integrationPepperID = "integration-v1"

func openIntegrationDB(t *testing.T, envName string) (*postgres.DB, auth.Pepper) {
	t.Helper()
	dsn := os.Getenv(envName)
	if dsn == "" {
		t.Skipf("requires explicit %s", envName)
	}
	db, err := postgres.Open(t.Context(), postgres.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	t.Cleanup(db.Close)
	migrations, err := migrate.LoadEmbedded()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if err := (migrate.Runner{Pool: db.Pool()}).Run(t.Context(), migrations); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return db, auth.Pepper{ID: integrationPepperID, Key: []byte("01234567890123456789012345678901")}
}

func integrationIDs(t *testing.T) (domain.TenantID, domain.ProjectID, domain.PrincipalID) {
	t.Helper()
	newID := func() domain.UUID {
		id, err := domain.NewUUID()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	return domain.TenantID(newID()), domain.ProjectID(newID()), domain.PrincipalID(newID())
}

func integrationUUID(id domain.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte(id), Valid: true}
}

func TestPostgresAuthenticatorRejectsPersistedInactiveAndRevokedKeys(t *testing.T) {
	db, pepper := openIntegrationDB(t, "URBINO_TEST_POSTGRES_DSN")
	ctx := context.Background()
	tenantID, projectID, userID := integrationIDs(t)
	store, err := postgres.NewIdentityStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateTenant(ctx, domain.Tenant{ID: tenantID, Name: "auth-integration-tenant", Currency: domain.Currency("USD"), Status: domain.TenantActive, Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateProject(ctx, domain.Project{ID: projectID, Tenant: tenantID, Name: "auth-integration-project", Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureUser(ctx, tenantID, userID, "auth-integration-user"); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureProjectMember(ctx, tenantID, projectID, userID, "member"); err != nil {
		t.Fatal(err)
	}
	issued, err := (auth.Issuer{Current: pepper}).Issue(tenantID, projectID, userID, []domain.PermissionScope{domain.ScopeModelsRead}, []string{"model-a"}, time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAPIKey(ctx, issued); err != nil {
		t.Fatal(err)
	}
	authenticator, err := postgres.NewAuthenticator(db, pepper, nil)
	if err != nil {
		t.Fatal(err)
	}
	req := auth.Request{Header: http.Header{"Authorization": []string{"Bearer " + issued.Value}}, RemoteAddr: "127.0.0.1:4000"}
	if _, err := authenticator.AuthenticatePublic(ctx, req); err != nil {
		t.Fatalf("active persisted key rejected: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, `UPDATE api_keys SET status = 'revoked', revoked_at = $2 WHERE public_id = $1`, issued.Record.PublicID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := authenticator.AuthenticatePublic(ctx, req); !errors.Is(err, auth.ErrInvalidCredential) {
		t.Fatalf("persisted inactive key error = %v, want invalid credential", err)
	}
}

func TestPostgresAuthenticatorRejectsPersistedAdminRevocation(t *testing.T) {
	db, pepper := openIntegrationDB(t, "URBINO_TEST_POSTGRES_DSN")
	ctx := context.Background()
	principalUUID, err := domain.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	principalID := domain.PrincipalID(principalUUID)
	issued, err := (auth.Issuer{Current: pepper}).IssueAdmin([]domain.PermissionScope{domain.ScopeTenantsRead}, time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO admin_principals (id, display_name, status, scopes, version)
		VALUES ($1, 'auth-integration-admin', 'active', $2, 1)`, integrationUUID(principalUUID), []string{string(domain.ScopeTenantsRead)}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool().Exec(ctx, `
		INSERT INTO admin_tokens (id, principal_id, public_id, secret_digest, digest_key_id, expires_at, auth_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, integrationUUID(issued.Record.ID), integrationUUID(principalUUID), issued.Record.PublicID, issued.Record.Digest, issued.Record.PepperID, issued.Record.ExpiresAt, issued.Record.AuthVersion); err != nil {
		t.Fatal(err)
	}
	authenticator, err := postgres.NewAuthenticator(db, pepper, nil)
	if err != nil {
		t.Fatal(err)
	}
	req := auth.Request{Header: http.Header{"Authorization": []string{"Bearer " + issued.Value}}, RemoteAddr: "127.0.0.1:4001"}
	principal, err := authenticator.AuthenticateAdmin(ctx, req)
	if err != nil || principal.AdminID != principalID {
		t.Fatalf("active persisted admin token = %#v, err=%v", principal, err)
	}
	future := time.Now().UTC().Add(time.Hour)
	if _, err := db.Pool().Exec(ctx, `UPDATE admin_tokens SET revoked_at = $2 WHERE public_id = $1`, issued.Record.PublicID, future); err != nil {
		t.Fatal(err)
	}
	if _, err := authenticator.AuthenticateAdmin(ctx, req); !errors.Is(err, auth.ErrInvalidCredential) {
		t.Fatalf("persisted revoked admin token error = %v, want invalid credential", err)
	}
}

func TestPostgresBootstrapHasSingleConcurrentWinner(t *testing.T) {
	db, pepper := openIntegrationDB(t, "URBINO_TEST_POSTGRES_BOOTSTRAP_DSN")
	issuer := auth.Issuer{Current: auth.Pepper{ID: "bootstrap-v1", Key: pepper.Key}}
	const attempts = 8
	paths := make([]string, attempts)
	for i := range paths {
		paths[i] = filepath.Join(t.TempDir(), "bootstrap.secret")
	}
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := range paths {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			results <- postgres.BootstrapAdmin(context.Background(), db, issuer, path, time.Now().UTC())
		}(paths[i])
	}
	wg.Wait()
	close(results)
	winners := 0
	winnerPath := ""
	for err := range results {
		if err == nil {
			winners++
		}
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			winnerPath = path
			break
		}
	}
	if winners != 1 {
		t.Fatalf("bootstrap winners = %d, want one", winners)
	}
	content, err := os.ReadFile(winnerPath)
	if err != nil {
		t.Fatalf("read winner secret file: %v", err)
	}
	if len(content) < 32 {
		t.Fatalf("winner secret file too short: %d", len(content))
	}
	for _, path := range paths {
		if path != winnerPath {
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("loser output %q still exists, stat error=%v", path, err)
			}
		}
	}
}

func TestPostgresBootstrapRollbackRemovesOutput(t *testing.T) {
	dsn := os.Getenv("URBINO_TEST_POSTGRES_BOOTSTRAP_ROLLBACK_DSN")
	if dsn == "" {
		t.Skip("requires explicit URBINO_TEST_POSTGRES_BOOTSTRAP_ROLLBACK_DSN")
	}
	db, err := postgres.Open(t.Context(), postgres.Config{DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	path := filepath.Join(t.TempDir(), "bootstrap.secret")
	issuer := auth.Issuer{Current: auth.Pepper{ID: "bootstrap-v1", Key: []byte("01234567890123456789012345678901")}}
	if err := postgres.BootstrapAdmin(t.Context(), db, issuer, path, time.Now().UTC()); err == nil {
		t.Fatal("bootstrap unexpectedly succeeded without schema")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed bootstrap left secret output, stat error=%v", err)
	}
}
