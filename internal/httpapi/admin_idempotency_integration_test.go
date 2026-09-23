package httpapi_test

// Stage 03 management mutation idempotency HTTP integration tests. They drive
// the real management API over HTTP with a real PostgreSQL store and assert
// consumer-observable behaviour plus database invariants: the same
// Idempotency-Key with the same payload replays the persisted safe response
// without executing the mutation twice, the same key with a different payload
// is a 409 conflict, a known mutation failure rolls the claim back so the key
// stays reusable, and an API key replay never carries the plaintext secret.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"example.com/urbino/internal/auth"
	"example.com/urbino/internal/domain"
	"example.com/urbino/internal/httpapi"
	"example.com/urbino/internal/storage/migrate"
	"example.com/urbino/internal/storage/postgres"
)

const idempotencyPepperID = "admin-idempotency-v1"

// idempotencyHarness is one running management API backed by real PostgreSQL.
type idempotencyHarness struct {
	db               *postgres.DB
	store            *postgres.IdentityStore
	client           *http.Client
	baseURL          string
	token            string
	adminID          domain.PrincipalID
	tenantID         domain.TenantID
	projectID        domain.ProjectID
	userID           domain.PrincipalID
	lastCacheControl string
}

func newIdempotencyHarness(t *testing.T) *idempotencyHarness {
	t.Helper()
	dsn := os.Getenv("URBINO_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("requires explicit URBINO_TEST_POSTGRES_DSN")
	}
	ctx := context.Background()
	db, err := postgres.Open(ctx, postgres.Config{DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	migrations, err := migrate.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	if err := (migrate.Runner{Pool: db.Pool()}).Run(ctx, migrations); err != nil {
		t.Fatal(err)
	}
	pepper := auth.Pepper{ID: idempotencyPepperID, Key: []byte("01234567890123456789012345678901")}
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
	if err := store.CreateTenant(ctx, domain.Tenant{ID: tenantID, Name: "idem-http-tenant-" + tenantID.String(), Currency: domain.Currency("USD"), Status: domain.TenantActive, Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateProject(ctx, domain.Project{ID: projectID, Tenant: tenantID, Name: "idem-http-project-" + projectID.String(), Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureUser(ctx, tenantID, userID, "idem-http-user"); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureProjectMember(ctx, tenantID, projectID, userID, "member"); err != nil {
		t.Fatal(err)
	}
	adminUUID := newID()
	adminID := domain.PrincipalID(adminUUID)
	adminScopes := []domain.PermissionScope{domain.ScopeTenantsRead, domain.ScopeTenantsWrite, domain.ScopeKeysRead, domain.ScopeKeysWrite}
	adminScopeStrings := make([]string, 0, len(adminScopes))
	for _, scope := range adminScopes {
		adminScopeStrings = append(adminScopeStrings, string(scope))
	}
	if _, err := db.Pool().Exec(ctx, `INSERT INTO admin_principals (id, display_name, status, scopes, version) VALUES ($1, 'idem-http-admin', 'active', $2, 1)`, pgUUID(adminUUID), adminScopeStrings); err != nil {
		t.Fatal(err)
	}
	adminToken, err := (auth.Issuer{Current: pepper}).IssueAdmin(adminScopes, time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool().Exec(ctx, `INSERT INTO admin_tokens (id, principal_id, public_id, secret_digest, digest_key_id, expires_at, auth_version) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		pgUUID(adminToken.Record.ID), pgUUID(adminUUID), adminToken.Record.PublicID, adminToken.Record.Digest, adminToken.Record.PepperID, adminToken.Record.ExpiresAt, adminToken.Record.AuthVersion); err != nil {
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
	serveDone := make(chan error, 1)
	go func() { serveDone <- servers.Serve(serveCtx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-serveDone:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("serve: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("authenticated servers did not stop")
		}
	})
	harness := &idempotencyHarness{
		db:        db,
		store:     store,
		client:    &http.Client{Timeout: 2 * time.Second},
		baseURL:   "http://" + servers.AdminAddr().String(),
		token:     adminToken.Value,
		adminID:   adminID,
		tenantID:  tenantID,
		projectID: projectID,
		userID:    userID,
	}
	if status := getStatus(t, harness.client, harness.baseURL+"/admin/v1/capabilities", harness.token, nil); status != http.StatusOK {
		t.Fatalf("management api did not become ready, status=%d", status)
	}
	return harness
}

// post sends one management mutation with an explicit idempotency key.
func (h *idempotencyHarness) post(t *testing.T, path, idempotencyKey string, body []byte) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, h.baseURL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)
	resp, err := h.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	h.lastCacheControl = resp.Header.Get("Cache-Control")
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, data
}

func idempotentName(t *testing.T, prefix string) string {
	t.Helper()
	id, err := domain.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	return prefix + "-" + id.String()
}

// idempotencyRowFor reads the durable claim row of one HTTP idempotency key.
func idempotencyRowFor(t *testing.T, h *idempotencyHarness, operation, idempotencyKey string) (state string, status int, body []byte) {
	t.Helper()
	keyDigest := sha256.Sum256([]byte(idempotencyKey))
	var rawStatus sql.NullInt64
	err := h.db.Pool().QueryRow(t.Context(), `
		SELECT state, response_status, response_body
		FROM admin_idempotency
		WHERE principal_id = $1 AND operation = $2 AND key_digest = $3`,
		pgUUID(domain.UUID(h.adminID)), operation, keyDigest[:]).Scan(&state, &rawStatus, &body)
	if err != nil {
		t.Fatalf("read admin_idempotency row: %v", err)
	}
	if !rawStatus.Valid {
		t.Fatalf("completed claim %s has no response status", idempotencyKey)
	}
	return state, int(rawStatus.Int64), body
}

func idempotencyRowCountFor(t *testing.T, h *idempotencyHarness, operation, idempotencyKey string) int {
	t.Helper()
	keyDigest := sha256.Sum256([]byte(idempotencyKey))
	var count int
	if err := h.db.Pool().QueryRow(t.Context(), `
		SELECT count(*) FROM admin_idempotency
		WHERE principal_id = $1 AND operation = $2 AND key_digest = $3`,
		pgUUID(domain.UUID(h.adminID)), operation, keyDigest[:]).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestAdminAPITenantCreateReplaysSamePersistedResponse(t *testing.T) {
	h := newIdempotencyHarness(t)
	key := "http-idem-tenant-create-key"
	name := idempotentName(t, "idem-http-replay-tenant")
	requestBody, err := json.Marshal(map[string]string{"name": name, "currency": "USD"})
	if err != nil {
		t.Fatal(err)
	}
	status1, body1 := h.post(t, "/admin/v1/tenants", key, requestBody)
	if status1 != http.StatusCreated {
		t.Fatalf("first tenant create status=%d body=%s", status1, body1)
	}
	var created struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Currency string `json:"currency"`
		Version  int64  `json:"version"`
	}
	if err := json.Unmarshal(body1, &created); err != nil || created.ID == "" || created.Name != name {
		t.Fatalf("first tenant create body=%s err=%v", body1, err)
	}
	status2, body2 := h.post(t, "/admin/v1/tenants", key, requestBody)
	if status2 < 200 || status2 >= 300 {
		t.Fatalf("replay status=%d, want the persisted success response", status2)
	}
	var replayed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body2, &replayed); err != nil || replayed.ID != created.ID {
		t.Fatalf("replay body=%s err=%v, want the same tenant", body2, err)
	}
	status3, body3 := h.post(t, "/admin/v1/tenants", key, requestBody)
	if status3 != status2 || !bytes.Equal(body2, body3) {
		t.Fatalf("replayed response not stable: %d/%d bytes differ=%v", status3, status2, !bytes.Equal(body2, body3))
	}
	replayedTenantID, err := domain.ParseTenantID(replayed.ID)
	if err != nil {
		t.Fatalf("replay tenant id %q: %v", replayed.ID, err)
	}
	var tenantCount int
	if err := h.db.Pool().QueryRow(t.Context(),
		`SELECT count(*) FROM tenants WHERE id = $1`, pgUUID(domain.UUID(replayedTenantID))).Scan(&tenantCount); err != nil {
		t.Fatal(err)
	}
	if tenantCount != 1 {
		t.Fatalf("tenant rows = %d after replay, want exactly one", tenantCount)
	}
	var auditCount int
	if err := h.db.Pool().QueryRow(t.Context(),
		`SELECT count(*) FROM audit_events WHERE target_id = $1 AND result = 'success'`,
		pgUUID(domain.UUID(replayedTenantID))).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("audit events = %d after replay, want exactly one", auditCount)
	}
	state, status, body := idempotencyRowFor(t, h, "tenant.create", key)
	if state != "completed" || status != status2 || !bytes.Equal(body, body2) {
		t.Fatalf("persisted claim state=%q status=%d, served replay %d with a different body", state, status, status2)
	}
}

func TestAdminAPISameKeyDifferentPayloadReturnsConflict(t *testing.T) {
	h := newIdempotencyHarness(t)
	key := "http-idem-payload-conflict-key"
	firstName := idempotentName(t, "idem-http-conflict-a")
	secondName := idempotentName(t, "idem-http-conflict-b")
	firstBody, err := json.Marshal(map[string]string{"name": firstName, "currency": "USD"})
	if err != nil {
		t.Fatal(err)
	}
	secondBody, err := json.Marshal(map[string]string{"name": secondName, "currency": "USD"})
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := h.post(t, "/admin/v1/tenants", key, firstBody); status != http.StatusCreated {
		t.Fatalf("first tenant create status=%d", status)
	}
	status, body := h.post(t, "/admin/v1/tenants", key, secondBody)
	if status != http.StatusConflict {
		t.Fatalf("same key with different payload status=%d body=%s, want 409", status, body)
	}
	var envelope struct {
		Code      string            `json:"code"`
		RequestID string            `json:"request_id"`
		Retryable *bool             `json:"retryable"`
		Details   map[string]string `json:"details"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("conflict body is not a management error envelope: %s", body)
	}
	if envelope.Code == "" || envelope.RequestID == "" || envelope.Retryable == nil || envelope.Details == nil {
		t.Fatalf("conflict envelope missing required fields: %s", body)
	}
	var secondCount int
	if err := h.db.Pool().QueryRow(t.Context(),
		`SELECT count(*) FROM tenants WHERE name = $1`, secondName).Scan(&secondCount); err != nil {
		t.Fatal(err)
	}
	if secondCount != 0 {
		t.Fatalf("conflicting payload created %d tenant rows", secondCount)
	}
	state, _, _ := idempotencyRowFor(t, h, "tenant.create", key)
	if state != "completed" {
		t.Fatalf("conflict changed the completed claim state to %q", state)
	}
}

func TestAdminAPIAPIKeyReplayResponseHasNoPlaintextSecret(t *testing.T) {
	h := newIdempotencyHarness(t)
	key := "http-idem-apikey-replay-key"
	requestBody, err := json.Marshal(map[string]any{
		"tenant_id":  h.tenantID.String(),
		"project_id": h.projectID.String(),
		"user_id":    h.userID.String(),
		"scopes":     []string{"models:read"},
		"models":     []string{"model-a"},
		"expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	status1, body1 := h.post(t, "/admin/v1/api-keys", key, requestBody)
	if status1 != http.StatusCreated {
		t.Fatalf("first api key create status=%d body=%s", status1, body1)
	}
	if h.lastCacheControl != "no-store" {
		t.Fatalf("first api key response Cache-Control=%q, want no-store", h.lastCacheControl)
	}
	var created struct {
		Key      string `json:"key"`
		PublicID string `json:"public_id"`
	}
	if err := json.Unmarshal(body1, &created); err != nil || created.Key == "" || created.PublicID == "" {
		t.Fatalf("first api key create omitted one-time credential: %s", body1)
	}
	if !bytes.Contains(body1, []byte(created.Key)) {
		t.Fatal("first api key create did not disclose the one-time secret")
	}
	status2, body2 := h.post(t, "/admin/v1/api-keys", key, requestBody)
	if status2 < 200 || status2 >= 300 {
		t.Fatalf("api key replay status=%d body=%s", status2, body2)
	}
	var replayed struct {
		PublicID        string `json:"public_id"`
		Message         string `json:"message"`
		SecretAvailable *bool  `json:"secret_available"`
	}
	if err := json.Unmarshal(body2, &replayed); err != nil {
		t.Fatalf("api key replay body is not JSON: %s", body2)
	}
	if replayed.PublicID != created.PublicID {
		t.Fatalf("api key replay metadata public_id=%q, want %q", replayed.PublicID, created.PublicID)
	}
	if replayed.SecretAvailable == nil || *replayed.SecretAvailable {
		t.Fatal("api key replay must state the secret is no longer available")
	}
	if !strings.Contains(strings.ToLower(replayed.Message), "secret") {
		t.Fatalf("api key replay hint %q does not explain the missing secret", replayed.Message)
	}
	if bytes.Contains(body2, []byte(created.Key)) {
		t.Fatal("api key replay response contains the plaintext secret")
	}
	status3, body3 := h.post(t, "/admin/v1/api-keys", key, requestBody)
	if status3 != status2 || !bytes.Equal(body2, body3) {
		t.Fatalf("api key replay not stable: %d bytes differ=%v", status3, !bytes.Equal(body2, body3))
	}
	var keyCount int
	if err := h.db.Pool().QueryRow(t.Context(),
		`SELECT count(*) FROM api_keys WHERE public_id = $1`, created.PublicID).Scan(&keyCount); err != nil {
		t.Fatal(err)
	}
	if keyCount != 1 {
		t.Fatalf("api key rows = %d after replay, want exactly one", keyCount)
	}
	var auditCount int
	if err := h.db.Pool().QueryRow(t.Context(),
		`SELECT count(*) FROM audit_events WHERE action = 'api_key.create' AND target_id = (SELECT id FROM api_keys WHERE public_id = $1)`,
		created.PublicID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("api key audit events = %d after replay, want exactly one", auditCount)
	}
	state, status, body := idempotencyRowFor(t, h, "api_key.create", key)
	if state != "completed" || status != status2 || !bytes.Equal(body, body2) {
		t.Fatalf("persisted api key claim state=%q status=%d does not match the served replay", state, status)
	}
	if bytes.Contains(body, []byte(created.Key)) {
		t.Fatal("persisted api key replay body contains the plaintext secret")
	}
}

func TestAdminAPIFailedMutationReleasesClaimForRetry(t *testing.T) {
	h := newIdempotencyHarness(t)
	strangerID := domain.PrincipalID(func() domain.UUID {
		id, err := domain.NewUUID()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}())
	if err := h.store.EnsureUser(t.Context(), h.tenantID, strangerID, "idem-http-stranger"); err != nil {
		t.Fatal(err)
	}
	// The stranger exists but is not a project member: a known in-transaction
	// mutation failure the caller can observe as a stable client error.
	failedBody, err := json.Marshal(map[string]any{
		"tenant_id":  h.tenantID.String(),
		"project_id": h.projectID.String(),
		"user_id":    strangerID.String(),
		"scopes":     []string{"models:read"},
		"models":     []string{"model-a"},
		"expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	key := "http-idem-failed-create-key"
	status, body := h.post(t, "/admin/v1/api-keys", key, failedBody)
	if status < 400 || status >= 500 {
		t.Fatalf("non-member create status=%d body=%s, want a client error", status, body)
	}
	if rows := idempotencyRowCountFor(t, h, "api_key.create", key); rows != 0 {
		t.Fatalf("failed mutation left %d idempotency claims", rows)
	}
	var strangerKeys int
	if err := h.db.Pool().QueryRow(t.Context(),
		`SELECT count(*) FROM api_keys WHERE user_id = $1`, pgUUID(domain.UUID(strangerID))).Scan(&strangerKeys); err != nil {
		t.Fatal(err)
	}
	if strangerKeys != 0 {
		t.Fatalf("failed mutation created %d api key rows", strangerKeys)
	}

	// The same key is reusable for the corrected payload: the rollback
	// released the claim together with the failed mutation.
	retryBody, err := json.Marshal(map[string]any{
		"tenant_id":  h.tenantID.String(),
		"project_id": h.projectID.String(),
		"user_id":    h.userID.String(),
		"scopes":     []string{"models:read"},
		"models":     []string{"model-a"},
		"expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	retryStatus, retryResponse := h.post(t, "/admin/v1/api-keys", key, retryBody)
	if retryStatus != http.StatusCreated {
		t.Fatalf("retry after failure status=%d body=%s, want a fresh creation", retryStatus, retryResponse)
	}
	var created struct {
		Key      string `json:"key"`
		PublicID string `json:"public_id"`
	}
	if err := json.Unmarshal(retryResponse, &created); err != nil || created.Key == "" || created.PublicID == "" {
		t.Fatalf("retry response omitted the one-time credential: %s", retryResponse)
	}
	var retryKeyCount int
	if err := h.db.Pool().QueryRow(t.Context(),
		`SELECT count(*) FROM api_keys WHERE public_id = $1`, created.PublicID).Scan(&retryKeyCount); err != nil {
		t.Fatal(err)
	}
	if retryKeyCount != 1 {
		t.Fatalf("retry created %d api key rows, want exactly one", retryKeyCount)
	}
	state, _, _ := idempotencyRowFor(t, h, "api_key.create", key)
	if state != "completed" {
		t.Fatalf("retry claim state=%q, want completed", state)
	}
}

func TestAdminAPIAPIKeyReplaySurvivesExpiry(t *testing.T) {
	h := newIdempotencyHarness(t)
	key := "http-idem-apikey-expiry-replay-key"
	requestBody, err := json.Marshal(map[string]any{
		"tenant_id":  h.tenantID.String(),
		"project_id": h.projectID.String(),
		"user_id":    h.userID.String(),
		"scopes":     []string{"models:read"},
		"models":     []string{"model-a"},
		"expires_at": time.Now().UTC().Add(1200 * time.Millisecond).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	status1, body1 := h.post(t, "/admin/v1/api-keys", key, requestBody)
	if status1 != http.StatusCreated {
		t.Fatalf("initial api key create status=%d body=%s", status1, body1)
	}
	time.Sleep(1500 * time.Millisecond)
	status2, body2 := h.post(t, "/admin/v1/api-keys", key, requestBody)
	if status2 != http.StatusCreated {
		t.Fatalf("expired api key replay status=%d body=%s", status2, body2)
	}
	if bytes.Contains(body2, []byte(`"key"`)) {
		t.Fatal("expired api key replay returned a one-time secret field")
	}
}
