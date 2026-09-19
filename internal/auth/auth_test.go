package auth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	cache, err := NewRevocationCache(revocationStore{state: RevocationState{AuthVersion: issued.Record.AuthVersion, CheckedAt: now}}, 5*time.Second)
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
