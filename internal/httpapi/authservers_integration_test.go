package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"example.com/urbino/internal/auth"
	"example.com/urbino/internal/domain"
	"example.com/urbino/internal/httpapi"
	"example.com/urbino/internal/storage/migrate"
	"example.com/urbino/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestAuthenticatedServersUsePersistentPublicAndAdminCredentials(t *testing.T) {
	dsn := os.Getenv("URBINO_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("requires explicit URBINO_TEST_POSTGRES_DSN")
	}
	ctx := context.Background()
	db, err := postgres.Open(ctx, postgres.Config{DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migrations, err := migrate.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	if err := (migrate.Runner{Pool: db.Pool()}).Run(ctx, migrations); err != nil {
		t.Fatal(err)
	}
	pepper := auth.Pepper{ID: "listener-integration-v1", Key: []byte("01234567890123456789012345678901")}
	store, err := postgres.NewIdentityStore(db)
	if err != nil {
		t.Fatal(err)
	}
	newID := func() domain.UUID {
		id, err := domain.NewUUID()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	tenantID := domain.TenantID(newID())
	projectID := domain.ProjectID(newID())
	userID := domain.PrincipalID(newID())
	if err := store.CreateTenant(ctx, domain.Tenant{ID: tenantID, Name: "listener-integration-tenant", Currency: domain.Currency("USD"), Status: domain.TenantActive, Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateProject(ctx, domain.Project{ID: projectID, Tenant: tenantID, Name: "listener-integration-project", Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureUser(ctx, tenantID, userID, "listener-integration-user"); err != nil {
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
	adminUUID := newID()
	admin := domain.PrincipalID(adminUUID)
	adminScopes := []domain.PermissionScope{domain.ScopeTenantsRead, domain.ScopeTenantsWrite, domain.ScopeKeysRead, domain.ScopeKeysWrite}
	adminToken, err := (auth.Issuer{Current: pepper}).IssueAdmin(adminScopes, time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	adminScopeStrings := make([]string, 0, len(adminScopes))
	for _, scope := range adminScopes {
		adminScopeStrings = append(adminScopeStrings, string(scope))
	}
	if _, err := db.Pool().Exec(ctx, `INSERT INTO admin_principals (id, display_name, status, scopes, version) VALUES ($1, 'listener-integration-admin', 'active', $2, 1)`, pgUUID(adminUUID), adminScopeStrings); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool().Exec(ctx, `INSERT INTO admin_tokens (id, principal_id, public_id, secret_digest, digest_key_id, expires_at, auth_version) VALUES ($1, $2, $3, $4, $5, $6, $7)`, pgUUID(adminToken.Record.ID), pgUUID(adminUUID), adminToken.Record.PublicID, adminToken.Record.Digest, adminToken.Record.PepperID, adminToken.Record.ExpiresAt, adminToken.Record.AuthVersion); err != nil {
		t.Fatal(err)
	}
	backupAdminToken, err := (auth.Issuer{Current: pepper}).IssueAdmin(adminScopes, time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool().Exec(ctx, `INSERT INTO admin_tokens (id, principal_id, public_id, secret_digest, digest_key_id, expires_at, auth_version) VALUES ($1, $2, $3, $4, $5, $6, $7)`, pgUUID(backupAdminToken.Record.ID), pgUUID(adminUUID), backupAdminToken.Record.PublicID, backupAdminToken.Record.Digest, backupAdminToken.Record.PepperID, backupAdminToken.Record.ExpiresAt, backupAdminToken.Record.AuthVersion); err != nil {
		t.Fatal(err)
	}
	authenticator, err := postgres.NewAuthenticator(db, pepper, nil)
	if err != nil {
		t.Fatal(err)
	}
	adminAPI, err := httpapi.NewAdminAPI(httpapi.AdminDeps{
		Store:             store,
		Issuer:            auth.Issuer{Current: pepper},
		AuthenticateAdmin: authenticator.AuthenticateAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	servers, err := httpapi.NewAuthenticatedServersWithAdminAPI("127.0.0.1:0", "127.0.0.1:0", authenticator.AuthenticatePublic, authenticator.AuthenticateAdmin, adminAPI)
	if err != nil {
		t.Fatal(err)
	}
	if err := servers.Listen(); err != nil {
		t.Fatal(err)
	}
	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	serveDone := make(chan error, 1)
	go func() { serveDone <- servers.Serve(serveCtx) }()
	defer func() {
		cancel()
		select {
		case err := <-serveDone:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("serve: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("authenticated servers did not stop")
		}
	}()

	client := &http.Client{Timeout: 2 * time.Second}
	publicURL := "http://" + servers.PublicAddr().String() + httpapi.PublicIdentityPath
	adminURL := "http://" + servers.AdminAddr().String() + httpapi.AdminIdentityPath
	if status := getStatus(t, client, publicURL, issued.Value, nil); status != http.StatusOK {
		t.Fatalf("public persistent key status=%d", status)
	}
	if status := getStatus(t, client, adminURL, issued.Value, nil); status != http.StatusUnauthorized {
		t.Fatalf("public key on admin listener status=%d", status)
	}
	if status := getStatus(t, client, adminURL, adminToken.Value, nil); status != http.StatusOK {
		t.Fatalf("admin persistent token status=%d for admin %s", status, admin)
	}
	if status := getStatus(t, client, publicURL, adminToken.Value, nil); status != http.StatusUnauthorized {
		t.Fatalf("admin token on public listener status=%d", status)
	}
	if status := getStatus(t, client, "http://"+servers.AdminAddr().String()+"/admin/v1/tenants", adminToken.Value, nil); status != http.StatusOK {
		t.Fatalf("admin tenant list status=%d", status)
	}
	keyRequest, err := json.Marshal(map[string]any{
		"tenant_id":  tenantID.String(),
		"project_id": projectID.String(),
		"user_id":    userID.String(),
		"scopes":     []string{"models:read"},
		"models":     []string{"model-a"},
		"expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	createStatus, createBody := requestBody(t, client, "http://"+servers.AdminAddr().String()+"/admin/v1/api-keys", adminToken.Value, http.MethodPost, keyRequest)
	if createStatus != http.StatusCreated {
		t.Fatalf("admin key create status=%d", createStatus)
	}
	var created struct {
		Key      string `json:"key"`
		PublicID string `json:"public_id"`
	}
	if err := json.Unmarshal(createBody, &created); err != nil || created.Key == "" || created.PublicID == "" {
		t.Fatalf("admin key create response omitted one-time key metadata")
	}
	listURL := "http://" + servers.AdminAddr().String() + "/admin/v1/api-keys?tenant_id=" + tenantID.String() + "&project_id=" + projectID.String()
	listStatus, listBody := requestBody(t, client, listURL, adminToken.Value, http.MethodGet, nil)
	if listStatus != http.StatusOK {
		t.Fatalf("admin key list status=%d", listStatus)
	}
	if bytes.Contains(listBody, []byte(created.Key)) {
		t.Fatalf("admin key list returned the one-time secret")
	}
	revokeRequest, err := json.Marshal(map[string]string{"tenant_id": tenantID.String(), "project_id": projectID.String()})
	if err != nil {
		t.Fatal(err)
	}
	revokeURL := "http://" + servers.AdminAddr().String() + "/admin/v1/api-keys/" + created.PublicID + "/revoke"
	if status, _ := requestBody(t, client, revokeURL, adminToken.Value, http.MethodPost, revokeRequest); status != http.StatusOK {
		t.Fatalf("admin key revoke status=%d", status)
	}
	if status := getStatus(t, client, publicURL, created.Key, nil); status != http.StatusUnauthorized {
		t.Fatalf("revoked managed key status=%d", status)
	}
	adminRevokeURL := "http://" + servers.AdminAddr().String() + "/admin/v1/admin-tokens/" + adminToken.Record.PublicID + "/revoke"
	if status, _ := requestBody(t, client, adminRevokeURL, adminToken.Value, http.MethodPost, nil); status != http.StatusOK {
		t.Fatalf("admin token revoke status=%d", status)
	}
	if status := getStatus(t, client, adminURL, adminToken.Value, nil); status != http.StatusUnauthorized {
		t.Fatalf("revoked admin token status=%d", status)
	}
	if status := getStatus(t, client, publicURL, issued.Value, map[string]string{"api_key": issued.Value}); status != http.StatusUnauthorized {
		t.Fatalf("query key conflict status=%d", status)
	}
	var auditCount int
	if err := db.Pool().QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE tenant_id = $1 AND action IN ('api_key.create', 'api_key.revoke')`, pgUUID(domain.UUID(tenantID))).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount < 3 {
		t.Fatalf("api key audit events = %d, want create/revoke events", auditCount)
	}
}

func getStatus(t *testing.T, client *http.Client, endpoint, credential string, query map[string]string) int {
	t.Helper()
	for attempt := 0; attempt < 50; attempt++ {
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+credential)
		if len(query) != 0 {
			values := req.URL.Query()
			for key, value := range query {
				values.Set(key, value)
			}
			req.URL.RawQuery = values.Encode()
		}
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			return resp.StatusCode
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("request to %s did not become reachable", endpoint)
	return 0
}

func requestBody(t *testing.T, client *http.Client, endpoint, credential, method string, body []byte) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+credential)
	if method != http.MethodGet {
		req.Header.Set("Idempotency-Key", endpoint+":"+method)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, data
}

func pgUUID(id domain.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte(id), Valid: true}
}
