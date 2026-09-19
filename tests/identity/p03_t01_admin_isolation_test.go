package identity_test

// P03-T01 — public key must fail against the admin authentication path on
// any listener, and admin tokens must never authenticate on the public path.
// Admin identity is a separate RBAC and is never derivable from a public
// principal. Socket-level listener separation (two net/http servers) is the
// main agent's e2e assembly; this file proves the in-process boundary the
// listeners must both sit behind.

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/urbino/tests/identity"
)

// TestP03T01PublicKeyRejectedOnAdminPath: a freshly issued valid public key
// presented to the admin authenticator must be rejected, via Bearer and via
// the protocol key headers alike.
func TestP03T01PublicKeyRejectedOnAdminPath(t *testing.T) {
	h := newHarness(t)
	issued, _, _ := issueKey(t, h, []string{"models:read"}, []string{"model-alpha"}, h.clk.Now().Add(time.Hour))

	cases := map[string]*identity.Request{
		"bearer":         bearerReq("POST", "/admin/v1/tenants", withBearer(nil, issued.Secret), nil, "203.0.113.10:4444"),
		"x-api-key":      bearerReq("POST", "/admin/v1/tenants", headerOf("x-api-key", issued.Secret), nil, "203.0.113.10:4444"),
		"x-goog-api-key": bearerReq("POST", "/admin/v1/tenants", headerOf("x-goog-api-key", issued.Secret), nil, "203.0.113.10:4444"),
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			p, err := h.sys.AuthenticateAdmin(context.Background(), req)
			if err == nil {
				t.Fatalf("public key authenticated on admin path: admin principal %+v", p)
			}
			if !errors.Is(err, identity.ErrUnauthorized) {
				t.Fatalf("rejection must be unauthorized class, got: %v", err)
			}
		})
	}
}

// TestP03T01AdminTokenRejectedOnPublicPath: a valid admin token must never
// authenticate as a public principal.
func TestP03T01AdminTokenRejectedOnPublicPath(t *testing.T) {
	h := newHarness(t)
	adminID := newID(t)
	cred, err := h.sys.BootstrapAdmin(context.Background(), identity.BootstrapRequest{
		AdminID: adminID,
		Scopes:  []string{"tenants:read", "tenants:write"},
	})
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}

	cases := map[string]*identity.Request{
		"bearer":    bearerReq("POST", "/v1/chat/completions", withBearer(nil, cred.Token), nil, "198.51.100.7:5555"),
		"x-api-key": bearerReq("POST", "/v1/chat/completions", headerOf("x-api-key", cred.Token), nil, "198.51.100.7:5555"),
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			p, err := h.sys.AuthenticatePublic(context.Background(), req)
			if err == nil {
				t.Fatalf("admin token authenticated on public path: principal %+v", p)
			}
			if !errors.Is(err, identity.ErrUnauthorized) {
				t.Fatalf("rejection must be unauthorized class, got: %v", err)
			}
		})
	}
}

// TestP03T01IndependentPrefixes: admin credentials must not reuse the public
// key format, so the two families are distinguishable before any lookup.
func TestP03T01IndependentPrefixes(t *testing.T) {
	h := newHarness(t)
	adminID := newID(t)
	cred, err := h.sys.BootstrapAdmin(context.Background(), identity.BootstrapRequest{
		AdminID: adminID,
		Scopes:  []string{"tenants:read"},
	})
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	if cred.Token == "" {
		t.Fatal("bootstrap must return a token")
	}
	if len(cred.Token) >= len(identity.KeyPrefix) && cred.Token[:len(identity.KeyPrefix)] == identity.KeyPrefix {
		t.Fatal("admin token must not carry the public key prefix gw_live_")
	}
}

// TestP03T01PublicPrincipalLacksAdminScopes: an authenticated public
// principal must hold no administrative authority — the admin scope check
// rejects it, so no request-body or header trick can promote a tenant role.
func TestP03T01PublicPrincipalLacksAdminScopes(t *testing.T) {
	h := newHarness(t)
	issued, _, _ := issueKey(t, h, []string{"models:read"}, []string{"model-alpha"}, h.clk.Now().Add(time.Hour))
	p := authPublic(t, h, bearerReq("POST", "/v1/chat/completions", withBearer(nil, issued.Secret), nil, "198.51.100.7:5555"))

	// Administrative authority names are not part of the public scope
	// vocabulary: an unknown scope is denied, and so is any write scope the
	// key was never granted.
	for _, adminScope := range []string{"tenants:write", "admin:keys:write", "admin:tenants:write"} {
		if err := h.sys.CheckPublicScope(p, adminScope); err == nil {
			t.Fatalf("public principal granted admin scope %q", adminScope)
		}
	}
	// And the admin scope checker itself must reject public principal data:
	if err := h.sys.CheckAdminScope(identity.AdminPrincipal{AdminID: p.UserID, Scopes: p.Scopes}, "admin:keys:read"); err == nil {
		t.Fatal("admin scope check accepted a principal derived from public key fields")
	}
}
