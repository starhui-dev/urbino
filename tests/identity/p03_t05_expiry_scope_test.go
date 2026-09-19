package identity_test

// P03-T05 — an expired key or a missing scope must be rejected before any
// dispatch decision, and admin scope enforcement must refuse escalation.

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/urbino/tests/identity"
)

// TestP03T05ExpiredKeyRejected: strictly after expires_at (and at the
// deadline itself, read conservatively as an inclusive expiry) the key must
// fail with ErrExpired; one tick before the deadline it still authenticates.
func TestP03T05ExpiredKeyRejected(t *testing.T) {
	h := newHarness(t)
	expires := h.clk.Now().Add(time.Hour)
	issued, _, _ := issueKey(t, h, []string{"models:invoke"}, []string{"model-alpha"}, expires)
	req := func() *identity.Request {
		return bearerReq("POST", "/v1/chat/completions", withBearer(nil, issued.Secret), nil, "198.51.100.7:5555")
	}

	h.clk.Advance(time.Hour - time.Millisecond)
	authPublic(t, h, req()) // still valid inside the window

	h.clk.Advance(time.Millisecond) // now == expires_at
	if p, err := h.sys.AuthenticatePublic(context.Background(), req()); err == nil {
		t.Fatalf("key accepted at its expiry deadline: %+v", p)
	} else if !errors.Is(err, identity.ErrExpired) {
		t.Fatalf("expected ErrExpired at deadline, got: %v", err)
	}

	h.clk.Advance(time.Minute) // well past expiry
	p, err := h.sys.AuthenticatePublic(context.Background(), req())
	if err == nil {
		t.Fatalf("expired key authenticated: %+v", p)
	}
	if !errors.Is(err, identity.ErrExpired) {
		t.Fatalf("expected ErrExpired, got: %v", err)
	}
	if !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("expiry must stay in the unauthorized class, got: %v", err)
	}
}

// TestP03T05ExpiredKeyRejectedAcrossRestartLikeCacheStates: expiry must hold
// even when a valid cached snapshot exists from before the deadline.
func TestP03T05ExpiredKeyRejectedWithWarmCache(t *testing.T) {
	h := newHarness(t)
	expires := h.clk.Now().Add(time.Hour)
	issued, _, _ := issueKey(t, h, []string{"models:invoke"}, []string{"model-alpha"}, expires)
	req := bearerReq("POST", "/v1/chat/completions", withBearer(nil, issued.Secret), nil, "198.51.100.7:5555")

	authPublic(t, h, req) // warm cache inside window
	h.clk.Advance(2 * time.Hour)
	p, err := h.sys.AuthenticatePublic(context.Background(), req)
	if err == nil {
		t.Fatalf("cached snapshot masked expiry: %+v", p)
	}
	if !errors.Is(err, identity.ErrExpired) {
		t.Fatalf("expected ErrExpired, got: %v", err)
	}
}

// TestP03T05MissingScopeRejected: a key without the required scope is denied
// while its granted scopes keep working.
func TestP03T05MissingScopeRejected(t *testing.T) {
	h := newHarness(t)
	issued, _, _ := issueKey(t, h, []string{"usage:read"}, []string{"model-alpha"}, h.clk.Now().Add(time.Hour))
	p := authPublic(t, h, bearerReq("POST", "/v1/usage", withBearer(nil, issued.Secret), nil, "198.51.100.7:5555"))

	if err := h.sys.CheckPublicScope(p, "usage:read"); err != nil {
		t.Fatalf("granted scope refused: %v", err)
	}
	err := h.sys.CheckPublicScope(p, "models:invoke")
	if err == nil {
		t.Fatal("ungranted scope accepted")
	}
	if !errors.Is(err, identity.ErrScopeDenied) {
		t.Fatalf("expected ErrScopeDenied, got: %v", err)
	}
	if !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("scope denial must stay in the forbidden class, got: %v", err)
	}
}

// TestP03T05AdminScopeEscalationRejected: an admin token scoped for reads
// must not exercise write scopes — no self-promotion.
func TestP03T05AdminScopeEscalationRejected(t *testing.T) {
	h := newHarness(t)
	cred, err := h.sys.BootstrapAdmin(context.Background(), identity.BootstrapRequest{
		AdminID: newID(t),
		Scopes:  []string{"admin:keys:read"},
	})
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	p, err := h.sys.AuthenticateAdmin(context.Background(), bearerReq("GET", "/admin/v1/api-keys", withBearer(nil, cred.Token), nil, "203.0.113.10:4444"))
	if err != nil {
		t.Fatalf("AuthenticateAdmin: %v", err)
	}
	if err := h.sys.CheckAdminScope(p, "admin:keys:read"); err != nil {
		t.Fatalf("granted admin scope refused: %v", err)
	}
	if err := h.sys.CheckAdminScope(p, "admin:keys:write"); err == nil {
		t.Fatal("admin scope escalation accepted")
	} else if !errors.Is(err, identity.ErrScopeDenied) {
		t.Fatalf("expected ErrScopeDenied, got: %v", err)
	}
}
