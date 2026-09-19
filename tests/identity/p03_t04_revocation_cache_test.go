package identity_test

// P03-T04 — revocation must take effect within the declared cache window
// (at most 5 seconds), and the cache must fail closed: backend errors on a
// cache miss or after the window, and lost invalidation with a stale
// auth_version must all end in rejection, never in a stale allow.
//
// Freshness inside the window is the contract's explicit allowance: the TTL
// is the staleness upper bound. Two System instances sharing one backend
// approximate the multi-instance deployment the matrix describes; true
// multi-process behaviour is re-checked in the Valkey phase.

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/urbino/tests/identity"
)

// secondInstance builds another System over the same shared backend.
func secondInstance(t *testing.T, h *harness) identity.System {
	t.Helper()
	sys, err := identity.New(context.Background(), identity.Options{
		Clock:          h.clk,
		Peppers:        testPeppers(t),
		ActivePepperID: "pepper-v1",
		Backend:        h.backend,
	})
	if errors.Is(err, identity.ErrUnbound) {
		t.Skip("internal/auth not linked (SKIP is not a pass)")
	}
	if err != nil {
		t.Fatalf("second identity.New: %v", err)
	}
	return sys
}

// TestP03T04RevocationTTLBound: the configured window must be positive and
// never exceed the 5 second contract.
func TestP03T04RevocationTTLBound(t *testing.T) {
	h := newHarness(t)
	ttl := h.sys.RevocationTTL()
	if ttl <= 0 {
		t.Fatalf("revocation TTL must be positive, got %v", ttl)
	}
	if ttl > identity.MaxRevocationTTL {
		t.Fatalf("revocation TTL %v exceeds the %v contract upper bound", ttl, identity.MaxRevocationTTL)
	}
}

// TestP03T04RevokedKeyRejectedWithinWindow: while the cached state is fresh
// the key authenticates; after revocation plus the full cache window the key
// must be rejected on every instance sharing the cache — no stale allow may
// survive past the declared bound.
func TestP03T04RevokedKeyRejectedWithinWindow(t *testing.T) {
	h := newHarness(t)
	sys2 := secondInstance(t, h)
	issued, tenant, project := issueKey(t, h, []string{"models:read"}, []string{"model-alpha"}, h.clk.Now().Add(time.Hour))
	publicID := issued.Record.PublicID

	req := func() *identity.Request {
		return bearerReq("POST", "/v1/models", withBearer(nil, issued.Secret), nil, "198.51.100.7:5555")
	}
	// Warm the cache and confirm the fresh window allows authentication.
	authPublic(t, h, req())
	h.clk.Advance(h.sys.RevocationTTL() - time.Millisecond)
	authPublic(t, h, req())

	// Revoke via the second instance (the multi-instance deployment shape).
	if err := sys2.RevokeKey(context.Background(), identity.Scope{TenantID: tenant, ProjectID: project}, publicID, h.clk.Now()); err != nil {
		t.Fatalf("RevokeKey: %v", err)
	}

	// Hard upper bound: one nanosecond past the cache window no stale allow
	// may survive on either instance. The contract requires effective
	// rejection within the window; it may surface as ErrRevoked directly or
	// as the fail-closed ErrCacheUnavailable class (the implementation
	// removes the shared entry on revocation, so the verifier rejects on
	// the ensuing miss). A stale allow is always a failure.
	h.clk.Advance(time.Millisecond + time.Nanosecond)
	t.Run("instance_that_revoked", func(t *testing.T) {
		p, err := sys2.AuthenticatePublic(context.Background(), req())
		if err == nil {
			t.Fatalf("revoked key authenticated past cache window: %+v", p)
		}
		if !errors.Is(err, identity.ErrRevoked) && !errors.Is(err, identity.ErrCacheUnavailable) {
			t.Fatalf("revocation must reject with ErrRevoked or fail-closed, got: %v", err)
		}
	})
	t.Run("instance_with_warmed_cache", func(t *testing.T) {
		p, err := h.sys.AuthenticatePublic(context.Background(), req())
		if err == nil {
			t.Fatalf("stale cached allow survived past the window: %+v", p)
		}
		if !errors.Is(err, identity.ErrRevoked) && !errors.Is(err, identity.ErrCacheUnavailable) {
			t.Fatalf("revocation must reject with ErrRevoked or fail-closed, got: %v", err)
		}
	})
}

// TestP03T04RevocationPropagatesToSharedStore: after RevokeKey returns, the
// shared store must no longer report the key as active — the entry is
// removed or replaced by a revoked/advanced-version state — so other
// instances reject within the window instead of at an arbitrary later time.
func TestP03T04RevocationPropagatesToSharedStore(t *testing.T) {
	h := newHarness(t)
	issued, tenant, project := issueKey(t, h, []string{"models:read"}, []string{"model-alpha"}, h.clk.Now().Add(time.Hour))
	publicID := issued.Record.PublicID

	authPublic(t, h, bearerReq("POST", "/v1/models", withBearer(nil, issued.Secret), nil, "198.51.100.7:5555"))
	before, ok := h.backend.get(publicID)
	if !ok {
		t.Fatal("shared store must know the key after issue/authenticate")
	}
	if err := h.sys.RevokeKey(context.Background(), identity.Scope{TenantID: tenant, ProjectID: project}, publicID, h.clk.Now()); err != nil {
		t.Fatalf("RevokeKey: %v", err)
	}
	after, ok := h.backend.get(publicID)
	if ok {
		stillActive := after.Record.RevokedAt.IsZero() && after.AuthVersion == before.AuthVersion
		if stillActive {
			t.Fatal("revocation left the shared store's active state untouched")
		}
	}
}

// TestP03T04CacheErrorFailsClosed: a backend error must reject whenever the
// verifier would otherwise need fresh truth — for a cold key, and for a warm
// entry once the declared freshness window has passed.
func TestP03T04CacheErrorFailsClosed(t *testing.T) {
	h := newHarness(t)
	issued, _, _ := issueKey(t, h, []string{"models:read"}, []string{"model-alpha"}, h.clk.Now().Add(time.Hour))
	req := bearerReq("POST", "/v1/models", withBearer(nil, issued.Secret), nil, "198.51.100.7:5555")

	t.Run("cold_key_backend_error", func(t *testing.T) {
		h.backend.failGet = true
		defer func() { h.backend.failGet = false }()
		p, err := h.sys.AuthenticatePublic(context.Background(), req)
		if err == nil {
			t.Fatalf("backend error must fail closed, authenticated: %+v", p)
		}
		if !errors.Is(err, identity.ErrCacheUnavailable) {
			t.Fatalf("expected ErrCacheUnavailable, got: %v", err)
		}
	})

	// Even a warm entry does not buy availability: the verifier consults the
	// shared truth on every decision and refuses on backend error. That is
	// stricter than the 5 second staleness upper bound, which the contract
	// permits (rejecting early is never a stale allow).
	t.Run("warm_entry_backend_error", func(t *testing.T) {
		authPublic(t, h, req) // warm entry
		h.backend.failGet = true
		defer func() { h.backend.failGet = false }()
		p, err := h.sys.AuthenticatePublic(context.Background(), req)
		if err == nil {
			t.Fatalf("backend error must fail closed even with a warm entry: %+v", p)
		}
		if !errors.Is(err, identity.ErrCacheUnavailable) {
			t.Fatalf("expected ErrCacheUnavailable, got: %v", err)
		}
	})

	// Once the window has passed the verifier needs the backend again; a
	// backend error must now reject instead of serving the expired entry.
	t.Run("window_elapsed_backend_error", func(t *testing.T) {
		h.backend.failGet = true
		defer func() { h.backend.failGet = false }()
		h.clk.Advance(h.sys.RevocationTTL() + time.Millisecond)
		p, err := h.sys.AuthenticatePublic(context.Background(), req)
		if err == nil {
			t.Fatalf("expired cache entry plus backend error must fail closed: %+v", p)
		}
		if !errors.Is(err, identity.ErrCacheUnavailable) {
			t.Fatalf("expected ErrCacheUnavailable, got: %v", err)
		}
	})
}

// TestP03T04StaleVersionFailsClosed: if revocation propagation to the shared
// store is lost, the old snapshot's auth_version no longer matches the
// authoritative record and the verifier must reject.
func TestP03T04StaleVersionFailsClosed(t *testing.T) {
	h := newHarness(t)
	issued, tenant, project := issueKey(t, h, []string{"models:read"}, []string{"model-alpha"}, h.clk.Now().Add(time.Hour))
	req := bearerReq("POST", "/v1/models", withBearer(nil, issued.Secret), nil, "198.51.100.7:5555")

	authPublic(t, h, req) // snapshot cached

	// Lose the invalidation: the shared store's Delete is swallowed, so its
	// snapshot stays at the old auth_version while the authoritative record
	// has moved on.
	h.backend.suppressDel = true
	if err := h.sys.RevokeKey(context.Background(), identity.Scope{TenantID: tenant, ProjectID: project}, issued.Record.PublicID, h.clk.Now()); err != nil {
		t.Fatalf("RevokeKey: %v", err)
	}
	h.backend.suppressDel = false

	p, err := h.sys.AuthenticatePublic(context.Background(), req)
	if err == nil {
		t.Fatalf("stale shared state must not authenticate a revoked key: %+v", p)
	}
	if !errors.Is(err, identity.ErrRevoked) && !errors.Is(err, identity.ErrStaleVersion) {
		t.Fatalf("expected ErrRevoked or ErrStaleVersion, got: %v", err)
	}
}
