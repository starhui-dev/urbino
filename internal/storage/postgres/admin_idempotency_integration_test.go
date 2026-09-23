package postgres_test

// Stage 03 management mutation idempotency integration tests. They exercise
// the RunAdminMutation executor against a real PostgreSQL schema and assert
// consumer-observable outcomes plus durable invariants: replays serve the
// persisted safe response, payload mismatches are rejected, failures roll the
// claim back with the mutation, replay bodies never carry plaintext secrets,
// stale pending claims fail closed, and completed records recover without a
// second execution. Direct SQL row injection stands in for a crashed process
// (the only seam a real database offers for commit-outcome recovery).

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"errors"
	"net/http"
	"testing"
	"time"

	"example.com/urbino/internal/auth"
	"example.com/urbino/internal/domain"
	"example.com/urbino/internal/ports"
	"example.com/urbino/internal/storage/postgres"
)

// mutationPrincipal inserts a dedicated active administrator row so the
// admin_idempotency foreign key resolves, and returns its principal id.
func mutationPrincipal(t *testing.T, db *postgres.DB) domain.PrincipalID {
	t.Helper()
	id, err := domain.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool().Exec(t.Context(), `
		INSERT INTO admin_principals (id, display_name, status, scopes, version)
		VALUES ($1, $2, 'active', $3, 1)`,
		integrationUUID(id), "idempotency-mutation-admin", []string{string(domain.ScopeTenantsWrite)}); err != nil {
		t.Fatal(err)
	}
	return domain.PrincipalID(id)
}

func mutationKeyDigest(raw string) []byte {
	digest := sha256.Sum256([]byte(raw))
	return digest[:]
}

func newMutationTenant(t *testing.T, name string) domain.Tenant {
	t.Helper()
	id, err := domain.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	return domain.Tenant{ID: domain.TenantID(id), Name: name, Currency: domain.Currency("USD"), Status: domain.TenantActive, Version: 1}
}

// insertIdempotencyRow seeds a claim row directly, simulating a process that
// crashed with the claim already durably written.
func insertIdempotencyRow(t *testing.T, db *postgres.DB, principal domain.PrincipalID, operation string, keyDigest, requestDigest []byte, response *ports.AdminMutationResponse) {
	t.Helper()
	if response == nil {
		if _, err := db.Pool().Exec(t.Context(), `
			INSERT INTO admin_idempotency (principal_id, operation, key_digest, request_digest)
			VALUES ($1, $2, $3, $4)`,
			integrationUUID(domain.UUID(principal)), operation, keyDigest, requestDigest); err != nil {
			t.Fatal(err)
		}
		return
	}
	if _, err := db.Pool().Exec(t.Context(), `
		INSERT INTO admin_idempotency (principal_id, operation, key_digest, request_digest,
		                               state, response_status, response_content_type, response_body, completed_at)
		VALUES ($1, $2, $3, $4, 'completed', $5, $6, $7, now())`,
		integrationUUID(domain.UUID(principal)), operation, keyDigest, requestDigest,
		response.Status, response.ContentType, response.Body); err != nil {
		t.Fatal(err)
	}
}

// idempotencyRow reads the durable claim state for one mutation key.
func idempotencyRow(t *testing.T, db *postgres.DB, principal domain.PrincipalID, operation string, keyDigest []byte) (state string, status sql.NullInt32, contentType sql.NullString, body []byte) {
	t.Helper()
	err := db.Pool().QueryRow(t.Context(), `
		SELECT state, response_status, response_content_type, response_body
		FROM admin_idempotency
		WHERE principal_id = $1 AND operation = $2 AND key_digest = $3`,
		integrationUUID(domain.UUID(principal)), operation, keyDigest).Scan(&state, &status, &contentType, &body)
	if err != nil {
		t.Fatalf("read admin_idempotency row: %v", err)
	}
	return state, status, contentType, body
}

func idempotencyRowCount(t *testing.T, db *postgres.DB, principal domain.PrincipalID, operation string, keyDigest []byte) int {
	t.Helper()
	var count int
	if err := db.Pool().QueryRow(t.Context(), `
		SELECT count(*) FROM admin_idempotency
		WHERE principal_id = $1 AND operation = $2 AND key_digest = $3`,
		integrationUUID(domain.UUID(principal)), operation, keyDigest).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// auditCountForTarget counts success audit records for one resource, proving
// a mutation executed (or did not re-execute) inside the transaction.
func auditCountForTarget(t *testing.T, db *postgres.DB, target domain.UUID) int {
	t.Helper()
	var count int
	if err := db.Pool().QueryRow(t.Context(),
		`SELECT count(*) FROM audit_events WHERE target_id = $1 AND result = 'success'`,
		integrationUUID(target)).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func requireTenantAbsent(t *testing.T, store *postgres.IdentityStore, tenant domain.Tenant) {
	t.Helper()
	if _, err := store.GetTenant(t.Context(), tenant.ID); !errors.Is(err, postgres.ErrNotFound) {
		t.Fatalf("tenant %s unexpectedly present, error = %v", tenant.ID, err)
	}
}

func TestAdminMutationReplaysPersistedSafeResponse(t *testing.T) {
	db, _ := openIntegrationDB(t, "URBINO_TEST_POSTGRES_DSN")
	store, err := postgres.NewIdentityStore(db)
	if err != nil {
		t.Fatal(err)
	}
	principal := mutationPrincipal(t, db)
	tenant := newMutationTenant(t, "idem-replay-tenant")
	keyDigest := mutationKeyDigest("p03-idem-replay-key")
	requestDigest := mutationKeyDigest("p03-idem-replay-request")
	firstBody := []byte(`{"created":true,"tenant_id":"` + tenant.ID.String() + `"}`)
	replayBody := []byte(`{"name":"idem-replay-tenant","tenant_id":"` + tenant.ID.String() + `"}`)
	calls := 0
	run := func() (ports.AdminMutationResult, error) {
		return store.RunAdminMutation(t.Context(), ports.AdminMutationRequest{
			Principal:     principal,
			Operation:     "tenant.create",
			KeyDigest:     keyDigest,
			RequestDigest: requestDigest,
		}, func(tx ports.AdminMutationTx) (ports.AdminMutationOutcome, error) {
			calls++
			if err := tx.CreateTenantFor(t.Context(), principal, tenant); err != nil {
				return ports.AdminMutationOutcome{}, err
			}
			return ports.AdminMutationOutcome{
				Response:       ports.AdminMutationResponse{Status: http.StatusCreated, ContentType: "application/json", Body: firstBody},
				ReplayResponse: ports.AdminMutationResponse{Status: http.StatusOK, ContentType: "application/json", Body: replayBody},
			}, nil
		})
	}
	result, err := run()
	if err != nil {
		t.Fatalf("first mutation: %v", err)
	}
	if calls != 1 || result.Replayed {
		t.Fatalf("first run executed=%d replayed=%v, want executed once, not replayed", calls, result.Replayed)
	}
	if result.Response.Status != http.StatusCreated || !bytes.Equal(result.Response.Body, firstBody) {
		t.Fatalf("first response = %+v, want 201 with creation body", result.Response)
	}
	if _, err := store.GetTenant(t.Context(), tenant.ID); err != nil {
		t.Fatalf("committed tenant unreadable: %v", err)
	}
	if state, status, contentType, body := idempotencyRow(t, db, principal, "tenant.create", keyDigest); state != "completed" ||
		!status.Valid || int(status.Int32) != http.StatusOK ||
		contentType.String != "application/json" || !bytes.Equal(body, replayBody) {
		t.Fatalf("persisted claim = %q %+v %+v %q, want completed replay response", state, status, contentType, body)
	}
	if audits := auditCountForTarget(t, db, domain.UUID(tenant.ID)); audits != 1 {
		t.Fatalf("audit events after first run = %d, want 1", audits)
	}

	result, err = run()
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if calls != 1 || !result.Replayed {
		t.Fatalf("replay executed=%d replayed=%v, want no second execution via replay", calls, result.Replayed)
	}
	if result.Response.Status != http.StatusOK ||
		result.Response.ContentType != "application/json" ||
		!bytes.Equal(result.Response.Body, replayBody) {
		t.Fatalf("replay response = %+v, want persisted safe replay response", result.Response)
	}
	if _, err := store.GetTenant(t.Context(), tenant.ID); err != nil {
		t.Fatalf("tenant lost after replay: %v", err)
	}
	if state, _, _, body := idempotencyRow(t, db, principal, "tenant.create", keyDigest); state != "completed" || !bytes.Equal(body, replayBody) {
		t.Fatalf("claim row mutated by replay: state=%q body=%q", state, body)
	}
	if audits := auditCountForTarget(t, db, domain.UUID(tenant.ID)); audits != 1 {
		t.Fatalf("audit events after replay = %d, want 1", audits)
	}
}

func TestAdminMutationRejectsSameKeyWithDifferentPayload(t *testing.T) {
	db, _ := openIntegrationDB(t, "URBINO_TEST_POSTGRES_DSN")
	store, err := postgres.NewIdentityStore(db)
	if err != nil {
		t.Fatal(err)
	}
	principal := mutationPrincipal(t, db)
	tenant := newMutationTenant(t, "idem-payload-guard-tenant")
	keyDigest := mutationKeyDigest("p03-idem-payload-key")
	replayBody := []byte(`{"tenant_id":"` + tenant.ID.String() + `"}`)
	run := func(requestDigest []byte) (int, error) {
		executions := 0
		_, err := store.RunAdminMutation(t.Context(), ports.AdminMutationRequest{
			Principal:     principal,
			Operation:     "tenant.create",
			KeyDigest:     keyDigest,
			RequestDigest: requestDigest,
		}, func(tx ports.AdminMutationTx) (ports.AdminMutationOutcome, error) {
			executions++
			if err := tx.CreateTenantFor(t.Context(), principal, tenant); err != nil {
				return ports.AdminMutationOutcome{}, err
			}
			return ports.AdminMutationOutcome{
				Response:       ports.AdminMutationResponse{Status: http.StatusCreated, ContentType: "application/json", Body: []byte(`{}`)},
				ReplayResponse: ports.AdminMutationResponse{Status: http.StatusOK, ContentType: "application/json", Body: replayBody},
			}, nil
		})
		return executions, err
	}
	if _, err := run(mutationKeyDigest("p03-idem-payload-request-a")); err != nil {
		t.Fatalf("first mutation: %v", err)
	}
	executions, err := run(mutationKeyDigest("p03-idem-payload-request-b"))
	if !errors.Is(err, postgres.ErrIdempotencyPayloadConflict) {
		t.Fatalf("payload mismatch error = %v, want ErrIdempotencyPayloadConflict", err)
	}
	if executions != 0 {
		t.Fatalf("payload mismatch executed the mutation %d times", executions)
	}
	// The rejected request changes nothing: the tenant from the first run stays
	// committed with its single audit record, and the completed claim keeps the
	// original replay response.
	if _, err := store.GetTenant(t.Context(), tenant.ID); err != nil {
		t.Fatalf("tenant lost after rejected payload mismatch: %v", err)
	}
	if audits := auditCountForTarget(t, db, domain.UUID(tenant.ID)); audits != 1 {
		t.Fatalf("audit events after payload mismatch = %d, want 1", audits)
	}
	if state, _, _, body := idempotencyRow(t, db, principal, "tenant.create", keyDigest); state != "completed" || !bytes.Equal(body, replayBody) {
		t.Fatalf("payload mismatch changed the completed claim: state=%q body=%q", state, body)
	}
}

func TestAdminMutationFailureRollsBackClaimStateAndAudit(t *testing.T) {
	db, _ := openIntegrationDB(t, "URBINO_TEST_POSTGRES_DSN")
	store, err := postgres.NewIdentityStore(db)
	if err != nil {
		t.Fatal(err)
	}
	principal := mutationPrincipal(t, db)
	tenant := newMutationTenant(t, "idem-rollback-tenant")
	keyDigest := mutationKeyDigest("p03-idem-rollback-key")
	plannedFailure := errors.New("planned mutation failure")
	_, err = store.RunAdminMutation(t.Context(), ports.AdminMutationRequest{
		Principal:     principal,
		Operation:     "tenant.create",
		KeyDigest:     keyDigest,
		RequestDigest: mutationKeyDigest("p03-idem-rollback-request-a"),
	}, func(tx ports.AdminMutationTx) (ports.AdminMutationOutcome, error) {
		if err := tx.CreateTenantFor(t.Context(), principal, tenant); err != nil {
			return ports.AdminMutationOutcome{}, err
		}
		return ports.AdminMutationOutcome{}, plannedFailure
	})
	if err == nil || errors.Is(err, postgres.ErrCommitOutcomeUnknown) {
		t.Fatalf("planned failure error = %v, want a definite failure", err)
	}
	requireTenantAbsent(t, store, tenant)
	if audits := auditCountForTarget(t, db, domain.UUID(tenant.ID)); audits != 0 {
		t.Fatalf("failed mutation left %d audit events", audits)
	}
	if rows := idempotencyRowCount(t, db, principal, "tenant.create", keyDigest); rows != 0 {
		t.Fatalf("failed mutation left %d idempotency claims", rows)
	}

	// The same key must be reusable after the rollback: no stale claim remains.
	tenant = newMutationTenant(t, "idem-rollback-retry-tenant")
	result, err := store.RunAdminMutation(t.Context(), ports.AdminMutationRequest{
		Principal:     principal,
		Operation:     "tenant.create",
		KeyDigest:     keyDigest,
		RequestDigest: mutationKeyDigest("p03-idem-rollback-request-b"),
	}, func(tx ports.AdminMutationTx) (ports.AdminMutationOutcome, error) {
		if err := tx.CreateTenantFor(t.Context(), principal, tenant); err != nil {
			return ports.AdminMutationOutcome{}, err
		}
		return ports.AdminMutationOutcome{
			Response:       ports.AdminMutationResponse{Status: http.StatusCreated, ContentType: "application/json", Body: []byte(`{"retried":true}`)},
			ReplayResponse: ports.AdminMutationResponse{Status: http.StatusOK, ContentType: "application/json", Body: []byte(`{"retried":true}`)},
		}, nil
	})
	if err != nil || result.Replayed {
		t.Fatalf("retry after failure: result=%+v err=%v, want fresh success", result, err)
	}
	if _, err := store.GetTenant(t.Context(), tenant.ID); err != nil {
		t.Fatalf("retried tenant unreadable: %v", err)
	}
	if state, _, _, _ := idempotencyRow(t, db, principal, "tenant.create", keyDigest); state != "completed" {
		t.Fatalf("retry claim state = %q, want completed", state)
	}
}

func TestAdminMutationAPIKeyReplayPersistsNoPlaintextSecret(t *testing.T) {
	db, pepper := openIntegrationDB(t, "URBINO_TEST_POSTGRES_DSN")
	store, err := postgres.NewIdentityStore(db)
	if err != nil {
		t.Fatal(err)
	}
	principal := mutationPrincipal(t, db)
	tenantID, projectID, userID := integrationIDs(t)
	if err := store.CreateTenant(t.Context(), domain.Tenant{ID: tenantID, Name: "idem-key-tenant", Currency: domain.Currency("USD"), Status: domain.TenantActive, Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateProject(t.Context(), domain.Project{ID: projectID, Tenant: tenantID, Name: "idem-key-project", Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureUser(t.Context(), tenantID, userID, "idem-key-user"); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureProjectMember(t.Context(), tenantID, projectID, userID, "member"); err != nil {
		t.Fatal(err)
	}
	issued, err := (auth.Issuer{Current: pepper}).Issue(tenantID, projectID, userID, []domain.PermissionScope{domain.ScopeModelsRead}, []string{"model-a"}, time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	keyDigest := mutationKeyDigest("p03-idem-apikey-key")
	requestDigest := mutationKeyDigest("p03-idem-apikey-request")
	firstBody := []byte(`{"key":"` + issued.Value + `","public_id":"` + issued.Record.PublicID + `"}`)
	replayBody := []byte(`{"public_id":"` + issued.Record.PublicID + `","secret_disclosed":"once"}`)
	calls := 0
	run := func() (ports.AdminMutationResult, error) {
		return store.RunAdminMutation(t.Context(), ports.AdminMutationRequest{
			Principal:     principal,
			Operation:     "api_key.create",
			KeyDigest:     keyDigest,
			RequestDigest: requestDigest,
		}, func(tx ports.AdminMutationTx) (ports.AdminMutationOutcome, error) {
			calls++
			if err := tx.CreateAPIKeyFor(t.Context(), principal, issued); err != nil {
				return ports.AdminMutationOutcome{}, err
			}
			return ports.AdminMutationOutcome{
				Response:       ports.AdminMutationResponse{Status: http.StatusCreated, ContentType: "application/json", Body: firstBody},
				ReplayResponse: ports.AdminMutationResponse{Status: http.StatusOK, ContentType: "application/json", Body: replayBody},
			}, nil
		})
	}
	result, err := run()
	if err != nil {
		t.Fatalf("first api key mutation: %v", err)
	}
	if result.Replayed || !bytes.Contains(result.Response.Body, []byte(issued.Value)) {
		t.Fatalf("first response must disclose the one-time secret once, got %+v", result.Response)
	}
	result, err = run()
	if err != nil {
		t.Fatalf("api key replay: %v", err)
	}
	if calls != 1 || !result.Replayed {
		t.Fatalf("api key replay executed=%d replayed=%v, want persisted replay", calls, result.Replayed)
	}
	if !bytes.Equal(result.Response.Body, replayBody) {
		t.Fatalf("api key replay body = %q, want persisted metadata-only body", result.Response.Body)
	}
	if bytes.Contains(result.Response.Body, []byte(issued.Value)) || bytes.Contains(result.Response.Body, []byte(issued.Secret)) {
		t.Fatal("api key replay response contains plaintext secret material")
	}
	if !bytes.Contains(result.Response.Body, []byte(issued.Record.PublicID)) {
		t.Fatal("api key replay response lost the key metadata")
	}
	_, status, _, body := idempotencyRow(t, db, principal, "api_key.create", keyDigest)
	if int(status.Int32) != http.StatusOK || !bytes.Equal(body, replayBody) {
		t.Fatalf("persisted api key replay = %d %q, want 200 %q", status.Int32, body, replayBody)
	}
	if bytes.Contains(body, []byte(issued.Value)) || bytes.Contains(body, []byte(issued.Secret)) {
		t.Fatal("persisted api key replay body contains plaintext secret material")
	}
	var keyCount int
	if err := db.Pool().QueryRow(t.Context(),
		`SELECT count(*) FROM api_keys WHERE public_id = $1`, issued.Record.PublicID).Scan(&keyCount); err != nil {
		t.Fatal(err)
	}
	if keyCount != 1 {
		t.Fatalf("api key rows = %d after replay, want exactly one", keyCount)
	}
	if audits := auditCountForTarget(t, db, issued.Record.ID); audits != 1 {
		t.Fatalf("api key audit events = %d after replay, want 1", audits)
	}
}

func TestAdminMutationStalePendingClaimBecomesSafeTerminalReplay(t *testing.T) {
	db, _ := openIntegrationDB(t, "URBINO_TEST_POSTGRES_DSN")
	store, err := postgres.NewIdentityStore(db)
	if err != nil {
		t.Fatal(err)
	}
	principal := mutationPrincipal(t, db)
	tenant := newMutationTenant(t, "idem-pending-tenant")
	keyDigest := mutationKeyDigest("p03-idem-pending-key")
	requestDigest := mutationKeyDigest("p03-idem-pending-request")
	otherRequestDigest := mutationKeyDigest("p03-idem-pending-different-request")
	insertIdempotencyRow(t, db, principal, "tenant.create", keyDigest, requestDigest, nil)
	calls := 0
	result, err := store.RunAdminMutation(t.Context(), ports.AdminMutationRequest{
		Principal:     principal,
		Operation:     "tenant.create",
		KeyDigest:     keyDigest,
		RequestDigest: otherRequestDigest,
	}, func(tx ports.AdminMutationTx) (ports.AdminMutationOutcome, error) {
		calls++
		return ports.AdminMutationOutcome{}, errors.New("mutation must not run behind a stale pending claim")
	})
	if err != nil {
		t.Fatalf("terminalize stale pending claim: %v", err)
	}
	if calls != 0 || !result.Replayed || result.Response.Status != http.StatusConflict {
		t.Fatalf("stale pending result=%+v calls=%d, want persisted 409 replay without execution", result, calls)
	}
	if !bytes.Contains(result.Response.Body, []byte(`"code":"idempotency_outcome_unknown"`)) {
		t.Fatalf("stale pending replay body=%s", result.Response.Body)
	}
	state, status, contentType, body := idempotencyRow(t, db, principal, "tenant.create", keyDigest)
	if state != "completed" || !status.Valid || status.Int32 != http.StatusConflict || contentType.String != "application/json" || !bytes.Equal(body, result.Response.Body) {
		t.Fatalf("terminalized pending row state=%q status=%+v content_type=%+v body=%q", state, status, contentType, body)
	}
	requireTenantAbsent(t, store, tenant)

	result, err = store.RunAdminMutation(t.Context(), ports.AdminMutationRequest{
		Principal:     principal,
		Operation:     "tenant.create",
		KeyDigest:     keyDigest,
		RequestDigest: otherRequestDigest,
	}, func(tx ports.AdminMutationTx) (ports.AdminMutationOutcome, error) {
		calls++
		return ports.AdminMutationOutcome{}, errors.New("terminal replay must not execute")
	})
	if err != nil || !result.Replayed || calls != 0 || !bytes.Equal(result.Response.Body, body) {
		t.Fatalf("second terminal replay result=%+v calls=%d err=%v", result, calls, err)
	}
}

func TestAdminMutationRecoversCompletedOutcomeWithoutRerun(t *testing.T) {
	db, _ := openIntegrationDB(t, "URBINO_TEST_POSTGRES_DSN")
	store, err := postgres.NewIdentityStore(db)
	if err != nil {
		t.Fatal(err)
	}
	principal := mutationPrincipal(t, db)
	tenant := newMutationTenant(t, "idem-crash-tenant")
	keyDigest := mutationKeyDigest("p03-idem-crash-key")
	requestDigest := mutationKeyDigest("p03-idem-crash-request")
	persisted := &ports.AdminMutationResponse{Status: http.StatusOK, ContentType: "application/json", Body: []byte(`{"recovered":true,"tenant_id":"` + tenant.ID.String() + `"}`)}
	insertIdempotencyRow(t, db, principal, "tenant.create", keyDigest, requestDigest, persisted)
	// The transaction that completed the claim also committed the tenant: this
	// is the durable state a crashed caller wakes up to.
	if err := store.CreateTenant(t.Context(), tenant); err != nil {
		t.Fatal(err)
	}
	calls := 0
	result, err := store.RunAdminMutation(t.Context(), ports.AdminMutationRequest{
		Principal:     principal,
		Operation:     "tenant.create",
		KeyDigest:     keyDigest,
		RequestDigest: requestDigest,
	}, func(tx ports.AdminMutationTx) (ports.AdminMutationOutcome, error) {
		calls++
		return ports.AdminMutationOutcome{}, errors.New("a completed claim must replay, not re-execute")
	})
	if err != nil {
		t.Fatalf("recovery after committed claim: %v", err)
	}
	if calls != 0 || !result.Replayed {
		t.Fatalf("recovery executed=%d replayed=%v, want replay without execution", calls, result.Replayed)
	}
	if result.Response.Status != persisted.Status ||
		result.Response.ContentType != persisted.ContentType ||
		!bytes.Equal(result.Response.Body, persisted.Body) {
		t.Fatalf("recovered response = %+v, want persisted %+v", result.Response, *persisted)
	}
	if _, err := store.GetTenant(t.Context(), tenant.ID); err != nil {
		t.Fatalf("recovered tenant unreadable: %v", err)
	}
	if audits := auditCountForTarget(t, db, domain.UUID(tenant.ID)); audits != 1 {
		t.Fatalf("audit events after recovery = %d, want the original 1", audits)
	}
}

func TestAdminMutationRejectsUnsafeReplayResponse(t *testing.T) {
	db, _ := openIntegrationDB(t, "URBINO_TEST_POSTGRES_DSN")
	store, err := postgres.NewIdentityStore(db)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		replay ports.AdminMutationResponse
	}{
		{"oversized body", ports.AdminMutationResponse{Status: http.StatusOK, ContentType: "application/json", Body: make([]byte, 64<<10+1)}},
		{"empty body", ports.AdminMutationResponse{Status: http.StatusOK, ContentType: "application/json", Body: nil}},
		{"status below success", ports.AdminMutationResponse{Status: 199, ContentType: "application/json", Body: []byte(`{}`)}},
		{"non-json content type", ports.AdminMutationResponse{Status: http.StatusOK, ContentType: "text/plain", Body: []byte(`{}`)}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			principal := mutationPrincipal(t, db)
			tenant := newMutationTenant(t, "idem-unsafe-tenant")
			keyDigest := mutationKeyDigest("p03-idem-unsafe-" + testCase.name)
			_, err := store.RunAdminMutation(t.Context(), ports.AdminMutationRequest{
				Principal:     principal,
				Operation:     "tenant.create",
				KeyDigest:     keyDigest,
				RequestDigest: mutationKeyDigest("p03-idem-unsafe-request"),
			}, func(tx ports.AdminMutationTx) (ports.AdminMutationOutcome, error) {
				if err := tx.CreateTenantFor(t.Context(), principal, tenant); err != nil {
					return ports.AdminMutationOutcome{}, err
				}
				return ports.AdminMutationOutcome{
					Response:       ports.AdminMutationResponse{Status: http.StatusCreated, ContentType: "application/json", Body: []byte(`{}`)},
					ReplayResponse: testCase.replay,
				}, nil
			})
			if err == nil || errors.Is(err, postgres.ErrCommitOutcomeUnknown) {
				t.Fatalf("unsafe replay response accepted: %v", err)
			}
			requireTenantAbsent(t, store, tenant)
			if audits := auditCountForTarget(t, db, domain.UUID(tenant.ID)); audits != 0 {
				t.Fatalf("unsafe response left %d audit events", audits)
			}
			if rows := idempotencyRowCount(t, db, principal, "tenant.create", keyDigest); rows != 0 {
				t.Fatalf("unsafe response left %d idempotency claims", rows)
			}
		})
	}
}

// Compile-time proof the production store implements the approved executor
// contract; a signature drift fails this file to build, not at runtime.
var _ ports.AdminMutationExecutor = (*postgres.IdentityStore)(nil)
