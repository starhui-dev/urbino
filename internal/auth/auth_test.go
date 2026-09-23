package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"example.com/urbino/internal/domain"
)

type revocationStore struct {
	state RevocationState
	err   error
}

func (s revocationStore) Lookup(context.Context, string) (RevocationState, error) {
	return s.state, s.err
}

func testIDs(t *testing.T) (domain.TenantID, domain.ProjectID, domain.PrincipalID) {
	t.Helper()
	first, err := domain.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := domain.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	third, err := domain.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	return domain.TenantID(first), domain.ProjectID(second), domain.PrincipalID(third)
}

func testIssuer() Issuer {
	return Issuer{Current: Pepper{ID: "pepper-v1", Key: []byte("01234567890123456789012345678901")}}
}

func TestIssueAndVerifyDoesNotStoreSecret(t *testing.T) {
	tenant, project, user := testIDs(t)
	issued, err := testIssuer().Issue(tenant, project, user, []domain.PermissionScope{domain.ScopeModelsRead}, []string{"model-a", "model-b"}, time.Now(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Secret == "" || issued.Value == "" || len(issued.Record.Digest) != 32 {
		t.Fatal("issue result is incomplete")
	}
	if string(issued.Record.Digest) == issued.Secret || issued.Record.PublicID == issued.Secret {
		t.Fatal("record contains the plaintext secret")
	}
	if err := Verify(issued.Record, issued.Secret, testIssuer().Current, time.Now()); err != nil {
		t.Fatalf("verify issued key: %v", err)
	}
	if err := Verify(issued.Record, "wrong-secret", testIssuer().Current, time.Now()); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("wrong secret error = %v", err)
	}
}

func TestExpiredRevokedAndScopeDenied(t *testing.T) {
	tenant, project, user := testIDs(t)
	now := time.Unix(100, 0)
	issued, err := testIssuer().Issue(tenant, project, user, []domain.PermissionScope{domain.ScopeModelsRead}, []string{"model-a"}, now, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := issued.Record.Authorize("model-b", domain.ScopeModelsRead, now); !errors.Is(err, ErrScopeDenied) {
		t.Fatalf("model scope error = %v", err)
	}
	if err := Verify(issued.Record, issued.Secret, testIssuer().Current, now.Add(2*time.Second)); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired error = %v", err)
	}
	revoked := now
	issued.Record.RevokedAt = &revoked
	if err := Verify(issued.Record, issued.Secret, testIssuer().Current, now); !errors.Is(err, ErrRevoked) {
		t.Fatalf("revoked error = %v", err)
	}
}

func TestParseHeadersRejectsConflictsAndQueryCredentials(t *testing.T) {
	good := http.Header{"Authorization": []string{"Bearer gw_live_public.secret"}}
	credential, err := ParseHeaders(good, url.Values{}, PublicListener)
	if err != nil || credential.Kind != GatewayCredential {
		t.Fatalf("public credential = %#v, err = %v", credential, err)
	}
	if _, err := ParseHeaders(http.Header{"Authorization": []string{"Bearer gw_live_a.b"}, "X-API-Key": []string{"gw_live_c.d"}}, url.Values{}, PublicListener); !errors.Is(err, ErrHeaderConflict) {
		t.Fatalf("conflict error = %v", err)
	}
	if _, err := ParseHeaders(http.Header{"Authorization": []string{"Bearer gw_live_a.b"}}, url.Values{"api_key": []string{"gw_live_c.d"}}, PublicListener); !errors.Is(err, ErrQueryCredential) {
		t.Fatalf("query error = %v", err)
	}
	for _, name := range []string{"KEY", "Api_Key", "X-API-Key", "Access_Token"} {
		if _, err := ParseHeaders(good, url.Values{name: []string{"gw_live_c.d"}}, PublicListener); !errors.Is(err, ErrQueryCredential) {
			t.Fatalf("query credential %q error = %v", name, err)
		}
	}
	if _, err := ParseHeaders(http.Header{"Authorization": []string{"Bearer gw_live_a.b"}}, url.Values{}, AdminListener); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("public key on admin error = %v", err)
	}
	if _, err := ParseHeaders(http.Header{"Authorization": []string{"Bearer gw_admin_a.b"}}, url.Values{}, PublicListener); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("admin key on public error = %v", err)
	}
	if _, err := ParseHeaders(http.Header{"Authorization": []string{"Bearer gw_admin_a.b"}}, url.Values{}, AdminListener); err != nil {
		t.Fatalf("admin credential rejected: %v", err)
	}
}

func TestRevocationCacheIsBoundedAndFailsClosed(t *testing.T) {
	tenant, project, user := testIDs(t)
	issued, err := testIssuer().Issue(tenant, project, user, []domain.PermissionScope{domain.ScopeModelsRead}, []string{"model-a"}, time.Now(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100, 0)
	cache, err := NewRevocationCache(revocationStore{state: RevocationState{AuthVersion: issued.Record.AuthVersion}}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Check(context.Background(), issued.Record.PublicID, issued.Record.AuthVersion, now); err != nil {
		t.Fatalf("cache check = %v", err)
	}
	if err := cache.Check(context.Background(), issued.Record.PublicID, issued.Record.AuthVersion, now.Add(6*time.Second)); err != nil {
		t.Fatalf("cache refresh = %v", err)
	}
	if _, err := NewRevocationCache(revocationStore{}, 6*time.Second); !errors.Is(err, ErrCacheUnavailable) {
		t.Fatalf("long ttl error = %v", err)
	}
	failed, err := NewRevocationCache(revocationStore{err: errors.New("backend down")}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := failed.Check(context.Background(), issued.Record.PublicID, issued.Record.AuthVersion, now); !errors.Is(err, ErrCacheUnavailable) {
		t.Fatalf("backend error = %v", err)
	}
	stale, err := NewRevocationCache(revocationStore{state: RevocationState{AuthVersion: issued.Record.AuthVersion, CheckedAt: now}}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := stale.Check(context.Background(), issued.Record.PublicID, issued.Record.AuthVersion, now.Add(6*time.Second)); !errors.Is(err, ErrCacheUnavailable) {
		t.Fatalf("stale cache state error = %v", err)
	}
}

func TestFailureLimiterAndExclusiveSecretOutput(t *testing.T) {
	limiter, err := NewFailureLimiter(2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100, 0)
	if !limiter.Allow("remote", now) || !limiter.Allow("remote", now.Add(time.Second)) || limiter.Allow("remote", now.Add(2*time.Second)) {
		t.Fatal("failure limiter window is not enforced")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "bootstrap.secret")
	if err := WriteSecretExclusive(path, "synthetic-secret"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("secret file mode = %v, err = %v", info.Mode().Perm(), err)
	}
	if err := WriteSecretExclusive(path, "replacement"); err == nil {
		t.Fatal("existing secret file was overwritten")
	}
}

func TestAdminTokenUsesIndependentPrefixAndDigest(t *testing.T) {
	issued, err := testIssuer().IssueAdmin([]domain.PermissionScope{domain.ScopeTenantsRead}, time.Unix(100, 0), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Value[:len("gw_admin_")] != "gw_admin_" {
		t.Fatalf("admin prefix = %q", issued.Value)
	}
	if err := VerifyAdmin(issued.Record, issued.Secret, testIssuer().Current, time.Unix(100, 0)); err != nil {
		t.Fatalf("verify admin token: %v", err)
	}
	if err := VerifyAdmin(issued.Record, "wrong", testIssuer().Current, time.Unix(100, 0)); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("wrong admin secret error = %v", err)
	}
}

func TestVerifyRejectsNonActiveAndFutureRevocation(t *testing.T) {
	tenant, project, user := testIDs(t)
	now := time.Unix(100, 0)
	issued, err := testIssuer().Issue(tenant, project, user, []domain.PermissionScope{domain.ScopeModelsRead}, []string{"model-a"}, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	issued.Record.Status = "expired"
	if err := Verify(issued.Record, issued.Secret, testIssuer().Current, now); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("non-active public key error = %v", err)
	}
	issued.Record.Status = "active"
	future := now.Add(time.Hour)
	issued.Record.RevokedAt = &future
	if err := Verify(issued.Record, issued.Secret, testIssuer().Current, now); !errors.Is(err, ErrRevoked) {
		t.Fatalf("future-revoked public key error = %v", err)
	}

	admin, err := testIssuer().IssueAdmin([]domain.PermissionScope{domain.ScopeTenantsRead}, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	admin.Record.RevokedAt = &future
	if err := VerifyAdmin(admin.Record, admin.Secret, testIssuer().Current, now); !errors.Is(err, ErrRevoked) {
		t.Fatalf("future-revoked admin token error = %v", err)
	}
}

func TestFailureLimiterReservesConcurrentAttemptsByHost(t *testing.T) {
	limiter, err := NewFailureLimiter(2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100, 0)
	start := make(chan struct{})
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			<-start
			if limiter.Begin(fmt.Sprintf("203.0.113.10:%d", 4000+port), now) {
				allowed.Add(1)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if got := allowed.Load(); got != 2 {
		t.Fatalf("concurrent reservations = %d, want 2", got)
	}

	limiter.RecordSuccess("203.0.113.10:9000", now)
	if !limiter.Begin("203.0.113.10:9001", now) {
		t.Fatal("successful authentication did not release reservation")
	}
	limiter.RecordFailure("203.0.113.10:9002", now)
	if limiter.Begin("203.0.113.10:9003", now) {
		t.Fatal("failed authentication did not consume shared host budget")
	}
	limiter.RecordSuccess("203.0.113.10:9004", now)
	if !limiter.Begin("203.0.113.10:9005", now.Add(time.Minute)) {
		t.Fatal("expired limiter window did not reset")
	}
}

// testClock is a deterministic clock.Clock the service tests advance by hand.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock(now time.Time) *testClock { return &testClock{now: now} }

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// memoryBackend is an in-memory RevocationBackend with no failure modes; the
// service-level tests here never exercise cache faults.
type memoryBackend struct {
	mu      sync.Mutex
	entries map[string]AuthState
}

func (b *memoryBackend) Get(_ context.Context, publicID string) (AuthState, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	state, ok := b.entries[publicID]
	return state, ok, nil
}

func (b *memoryBackend) Put(_ context.Context, publicID string, state AuthState, _ time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries[publicID] = state
	return nil
}

func (b *memoryBackend) Delete(_ context.Context, publicID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.entries, publicID)
	return nil
}

func newAdminService(t *testing.T, now time.Time) (*Service, *testClock) {
	t.Helper()
	clk := newTestClock(now)
	service, ttl, err := New(context.Background(), Options{
		Clock:          clk,
		Peppers:        map[string][]byte{"pepper-v1": []byte("01234567890123456789012345678901")},
		ActivePepperID: "pepper-v1",
		Backend:        &memoryBackend{entries: make(map[string]AuthState)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ttl <= 0 {
		t.Fatalf("revocation ttl = %v", ttl)
	}
	return service, clk
}

func newTestPrincipalID(t *testing.T) domain.PrincipalID {
	t.Helper()
	id, err := domain.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	return domain.PrincipalID(id)
}

func adminRequest(token, remoteAddr string) Request {
	return Request{
		Method:     http.MethodGet,
		Path:       "/admin/v1/session",
		Header:     http.Header{"Authorization": []string{"Bearer " + token}},
		Query:      url.Values{},
		RemoteAddr: remoteAddr,
	}
}

func adminTokenPublicID(t *testing.T, token string) string {
	t.Helper()
	publicID, _, err := splitCredential(token, adminPrefix)
	if err != nil {
		t.Fatalf("split admin token: %v", err)
	}
	return publicID
}

// TestVerifyRecordRejectsNonActiveStatusAndAnyRevocation pins the Authorize
// boundary: VerifyRecord backs it, so a record that is not exactly active, or
// that carries any revocation timestamp regardless of when it falls, must
// never authorize.
func TestVerifyRecordRejectsNonActiveStatusAndAnyRevocation(t *testing.T) {
	tenant, project, user := testIDs(t)
	now := time.Unix(5000, 0)
	issued, err := testIssuer().Issue(tenant, project, user, []domain.PermissionScope{domain.ScopeModelsRead}, []string{"model-a"}, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := issued.Record.Authorize("model-a", domain.ScopeModelsRead, now); err != nil {
		t.Fatalf("active record denied: %v", err)
	}
	for _, status := range []string{"", "expired", "disabled", "revoked", "Active"} {
		record := issued.Record
		record.Status = status
		if err := VerifyRecord(record, now); !errors.Is(err, ErrInvalidCredential) {
			t.Fatalf("status %q accepted by VerifyRecord: %v", status, err)
		}
		if err := record.Authorize("model-a", domain.ScopeModelsRead, now); !errors.Is(err, ErrInvalidCredential) {
			t.Fatalf("status %q authorized: %v", status, err)
		}
	}
	for name, at := range map[string]time.Time{"past": now.Add(-time.Hour), "present": now, "future": now.Add(time.Hour)} {
		record := issued.Record
		revoked := at
		record.RevokedAt = &revoked
		if err := VerifyRecord(record, now); !errors.Is(err, ErrRevoked) {
			t.Fatalf("%s revocation accepted by VerifyRecord: %v", name, err)
		}
		if err := record.Authorize("model-a", domain.ScopeModelsRead, now); !errors.Is(err, ErrRevoked) {
			t.Fatalf("%s revocation authorized: %v", name, err)
		}
	}
	expired := issued.Record
	expired.ExpiresAt = now
	if err := VerifyRecord(expired, now); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired record error = %v", err)
	}
	nonActiveAndRevoked := issued.Record
	nonActiveAndRevoked.Status = "revoked"
	future := now.Add(time.Hour)
	nonActiveAndRevoked.RevokedAt = &future
	if err := nonActiveAndRevoked.Authorize("model-a", domain.ScopeModelsRead, now); err == nil {
		t.Fatal("a record that is neither active nor unrevoked was authorized")
	}
}

// TestBootstrapAdminFixesScopesAndBoundsLifetime proves request content cannot
// change the fixed bootstrap authority and the credential remains bounded.
func TestBootstrapAdminFixesScopesAndBoundsLifetime(t *testing.T) {
	service, clk := newAdminService(t, time.Unix(1000, 0))
	adminID := newTestPrincipalID(t)
	cred, err := service.BootstrapAdmin(context.Background(), BootstrapRequest{
		AdminID: adminID,
		Scopes:  []string{string(domain.ScopeKeysWrite), string(domain.ScopeTenantsWrite), string(domain.ScopeModelsInvoke)},
	})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	principal, err := service.AuthenticateAdmin(context.Background(), adminRequest(cred.Token, "203.0.113.10:4444"))
	if err != nil {
		t.Fatalf("authenticate bootstrap credential: %v", err)
	}
	if principal.AdminID != adminID {
		t.Fatalf("bootstrap principal id = %v, want %v", principal.AdminID, adminID)
	}
	if err := service.CheckAdminScope(principal, string(domain.ScopeModelsInvoke)); !errors.Is(err, ErrScopeDenied) {
		t.Fatalf("bootstrap self-granted unsupported scope: %v", err)
	}
	for _, fixed := range bootstrapAdminScopes {
		if err := service.CheckAdminScope(principal, string(fixed)); err != nil {
			t.Fatalf("fixed bootstrap scope %q refused: %v", fixed, err)
		}
	}
	if len(principal.Scopes) != len(bootstrapAdminScopes) {
		t.Fatalf("bootstrap scopes = %v, want the fixed set %v", principal.Scopes, toScopeStrings(bootstrapAdminScopes))
	}

	clk.Advance(bootstrapAdminTokenTTL)
	if _, err := service.AuthenticateAdmin(context.Background(), adminRequest(cred.Token, "203.0.113.10:4444")); !errors.Is(err, ErrExpired) {
		t.Fatalf("bootstrap credential outlived its bounded lifetime: %v", err)
	}
}

// TestIssueAdminTokenBoundsLifetimeAndValidatesScopes proves additional admin
// tokens are bounded in lifetime, validated, and never widened past the
// issuing principal.
func TestIssueAdminTokenBoundsLifetimeAndValidatesScopes(t *testing.T) {
	service, clk := newAdminService(t, time.Unix(2000, 0))
	adminID := newTestPrincipalID(t)
	if _, err := service.BootstrapAdmin(context.Background(), BootstrapRequest{AdminID: adminID}); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	request := func(scopes []string, ttl time.Duration) IssueAdminTokenRequest {
		return IssueAdminTokenRequest{AdminID: adminID, Scopes: scopes, TTL: ttl}
	}
	if _, err := service.IssueAdminToken(context.Background(), request([]string{string(domain.ScopeKeysRead)}, 0)); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("zero ttl error = %v", err)
	}
	if _, err := service.IssueAdminToken(context.Background(), request([]string{string(domain.ScopeKeysRead)}, -time.Minute)); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("negative ttl error = %v", err)
	}
	if _, err := service.IssueAdminToken(context.Background(), request([]string{string(domain.ScopeKeysRead)}, maxAdminTokenTTL+time.Second)); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("over-long ttl error = %v", err)
	}
	if _, err := service.IssueAdminToken(context.Background(), request(nil, time.Hour)); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("empty scopes error = %v", err)
	}
	if _, err := service.IssueAdminToken(context.Background(), request([]string{"not-a-scope"}, time.Hour)); !errors.Is(err, ErrScopeDenied) {
		t.Fatalf("unknown scope error = %v", err)
	}
	if _, err := service.IssueAdminToken(context.Background(), request([]string{string(domain.ScopeModelsInvoke)}, time.Hour)); !errors.Is(err, ErrScopeDenied) {
		t.Fatalf("escalated scope error = %v", err)
	}
	if _, err := service.IssueAdminToken(context.Background(), IssueAdminTokenRequest{AdminID: newTestPrincipalID(t), Scopes: []string{string(domain.ScopeKeysRead)}, TTL: time.Hour}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown principal error = %v", err)
	}

	issued, err := service.IssueAdminToken(context.Background(), request([]string{string(domain.ScopeKeysRead), string(domain.ScopeKeysRead)}, time.Hour))
	if err != nil {
		t.Fatalf("issue admin token: %v", err)
	}
	principal, err := service.AuthenticateAdmin(context.Background(), adminRequest(issued.Token, "203.0.113.11:4444"))
	if err != nil {
		t.Fatalf("authenticate issued admin token: %v", err)
	}
	if err := service.CheckAdminScope(principal, string(domain.ScopeKeysRead)); err != nil {
		t.Fatalf("requested scope refused: %v", err)
	}
	if err := service.CheckAdminScope(principal, string(domain.ScopeTenantsRead)); !errors.Is(err, ErrScopeDenied) {
		t.Fatalf("token widened past its request: %v", err)
	}
	if len(principal.Scopes) != 1 {
		t.Fatalf("duplicate scopes stored: %v", principal.Scopes)
	}
	clk.Advance(time.Hour)
	if _, err := service.AuthenticateAdmin(context.Background(), adminRequest(issued.Token, "203.0.113.11:4444")); !errors.Is(err, ErrExpired) {
		t.Fatalf("admin token outlived its requested ttl: %v", err)
	}
}

// TestRevokeAdminTokenStopsAuthentication covers the emergency revocation
// path: only the owning principal may revoke, revocation is immediate
// regardless of the timestamp supplied, and unrelated tokens survive.
func TestRevokeAdminTokenStopsAuthentication(t *testing.T) {
	service, clk := newAdminService(t, time.Unix(3000, 0))
	adminID := newTestPrincipalID(t)
	bootstrap, err := service.BootstrapAdmin(context.Background(), BootstrapRequest{AdminID: adminID})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	extra, err := service.IssueAdminToken(context.Background(), IssueAdminTokenRequest{AdminID: adminID, Scopes: []string{string(domain.ScopeKeysRead)}, TTL: time.Hour})
	if err != nil {
		t.Fatalf("issue admin token: %v", err)
	}
	extraID := adminTokenPublicID(t, extra.Token)
	spare, err := service.IssueAdminToken(context.Background(), IssueAdminTokenRequest{AdminID: adminID, Scopes: []string{string(domain.ScopeKeysRead)}, TTL: time.Hour})
	if err != nil {
		t.Fatalf("issue spare admin token: %v", err)
	}

	if err := service.RevokeAdminToken(context.Background(), newTestPrincipalID(t), extraID, clk.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign principal revoked an admin token: %v", err)
	}
	if err := service.RevokeAdminToken(context.Background(), adminID, adminTokenPublicID(t, bootstrap.Token), clk.Now()); err != nil {
		t.Fatalf("revoke bootstrap admin token: %v", err)
	}
	if _, err := service.AuthenticateAdmin(context.Background(), adminRequest(bootstrap.Token, "203.0.113.12:4444")); !errors.Is(err, ErrRevoked) {
		t.Fatalf("revoked admin token authenticated: %v", err)
	}
	if err := service.RevokeAdminToken(context.Background(), adminID, adminTokenPublicID(t, bootstrap.Token), clk.Now()); !errors.Is(err, ErrRevoked) {
		t.Fatalf("repeated revocation error = %v", err)
	}
	if err := service.RevokeAdminToken(context.Background(), domain.PrincipalID{}, extraID, clk.Now()); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("zero principal error = %v", err)
	}
	if err := service.RevokeAdminToken(context.Background(), adminID, "", clk.Now()); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("empty token id error = %v", err)
	}
	if err := service.RevokeAdminToken(context.Background(), adminID, "missing-token-id", clk.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown admin token error = %v", err)
	}

	// A future-dated revocation still fails closed immediately.
	if err := service.RevokeAdminToken(context.Background(), adminID, extraID, clk.Now().Add(time.Hour)); err != nil {
		t.Fatalf("future-dated revocation: %v", err)
	}
	if _, err := service.AuthenticateAdmin(context.Background(), adminRequest(extra.Token, "203.0.113.12:4445")); !errors.Is(err, ErrRevoked) {
		t.Fatalf("future-dated revocation left the token usable: %v", err)
	}

	if err := service.RevokeAdminToken(context.Background(), adminID, adminTokenPublicID(t, spare.Token), clk.Now()); !errors.Is(err, ErrLastAdminToken) {
		t.Fatalf("revoking final usable admin token error = %v, want ErrLastAdminToken", err)
	}
}

// TestFailureLimiterEvictsExpiredAndOldestBuckets proves the limiter map is
// bounded: expired windows are swept, the oldest window is evicted when the
// map is full, and eviction never breaks reservation accounting.
func TestFailureLimiterEvictsExpiredAndOldestBuckets(t *testing.T) {
	limiter, err := NewFailureLimiter(1, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	limiter.max = 2
	now := time.Unix(4000, 0)
	if !limiter.Begin("203.0.113.1:1000", now) || !limiter.Begin("203.0.113.2:1000", now.Add(time.Second)) {
		t.Fatal("reservations inside capacity were refused")
	}
	if len(limiter.buckets) != 2 {
		t.Fatalf("tracked buckets = %d, want 2", len(limiter.buckets))
	}
	if !limiter.Begin("203.0.113.3:1000", now.Add(2*time.Second)) {
		t.Fatal("new source refused although an old window could be evicted")
	}
	if len(limiter.buckets) != 2 {
		t.Fatalf("bucket map grew past capacity: %d", len(limiter.buckets))
	}
	if _, tracked := limiter.buckets["203.0.113.1"]; tracked {
		t.Fatal("oldest window survived eviction")
	}
	if !limiter.Begin("203.0.113.1:1000", now.Add(3*time.Second)) {
		t.Fatal("evicted source could not reserve again")
	}
	if limiter.Begin("203.0.113.3:1000", now.Add(3*time.Second)) {
		t.Fatal("eviction lost an in-flight reservation")
	}

	expiring, err := NewFailureLimiter(5, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	expiring.max = 8
	for i := range 8 {
		if !expiring.Allow(fmt.Sprintf("198.51.100.%d:5000", i), now) {
			t.Fatalf("allow %d refused", i)
		}
	}
	if len(expiring.buckets) != 8 {
		t.Fatalf("tracked buckets = %d, want 8", len(expiring.buckets))
	}
	if !expiring.Allow("198.51.100.200:5000", now.Add(2*time.Second)) {
		t.Fatal("allow refused after the window expired")
	}
	if len(expiring.buckets) != 1 {
		t.Fatalf("expired windows were not swept: %d buckets left", len(expiring.buckets))
	}
}

func TestIssuerRejectsAdministratorScopesForPublicKeys(t *testing.T) {
	tenant, project, user := testIDs(t)
	_, err := testIssuer().Issue(tenant, project, user, []domain.PermissionScope{domain.ScopeKeysWrite}, []string{"model-a"}, time.Now().UTC(), time.Hour)
	if !errors.Is(err, ErrScopeDenied) {
		t.Fatalf("public key issuer accepted administrator scope: %v", err)
	}
}
