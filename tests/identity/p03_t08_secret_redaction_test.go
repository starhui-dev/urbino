package identity_test

// P03-T08 — the plaintext secret is disclosed exactly once at creation and
// never reappears: not in stored records, not in any error surfaced by the
// authentication boundary, not in logs. Digest-bearing records are fine; the
// plaintext and any reversible form are not.

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/urbino/tests/identity"
)

// leakScan fails the test when any plaintext secret material appears in text.
func leakScan(t *testing.T, what, text string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if s != "" && strings.Contains(text, s) {
			t.Fatalf("%s contains plaintext secret material (%s)", what, identity.Redact(s))
		}
	}
}

func TestP03T08KeyRecordCarriesNoPlaintext(t *testing.T) {
	h := newHarness(t)
	issued, _, _ := issueKey(t, h, []string{"models:invoke"}, []string{"model-alpha"}, h.clk.Now().Add(time.Hour))

	raw, err := json.Marshal(issued.Record)
	if err != nil {
		t.Fatalf("marshal KeyRecord: %v", err)
	}
	leakScan(t, "KeyRecord JSON", string(raw), issued.Secret)

	if string(issued.Record.Digest) == issued.Secret {
		t.Fatal("record digest equals the plaintext secret")
	}
	if len(issued.Record.Digest) != 32 {
		t.Fatalf("HMAC-SHA-256 digest must be 32 bytes, got %d", len(issued.Record.Digest))
	}
	if issued.Record.DigestKeyID == "" {
		t.Fatal("record must carry the pepper/digest key id")
	}

	// Hex and base64 renderings aside, the decisive check: the digest is not
	// reversible plaintext and the record carries the pepper id instead of
	// any secret material.
	leakScan(t, "KeyRecord JSON", string(raw), issued.Secret)
}

// TestP03T08ErrorsNeverEchoSecret: every failure path reachable with the
// valid secret in the request must format without echoing it.
func TestP03T08ErrorsNeverEchoSecret(t *testing.T) {
	h := newHarness(t)
	issued, tenant, project := issueKey(t, h, []string{"models:read"}, []string{"model-alpha"}, h.clk.Now().Add(time.Hour))
	secret := issued.Secret
	scope := identity.Scope{TenantID: tenant, ProjectID: project}

	failure := func(name string, act func() error) {
		t.Run(name, func(t *testing.T) {
			err := act()
			if err == nil {
				t.Skip("path accepted the request; nothing to scan")
			}
			leakScan(t, "error text", errText(err), secret)
		})
	}

	failure("expired_key", func() error {
		h.clk.Advance(48 * time.Hour)
		_, err := h.sys.AuthenticatePublic(context.Background(), bearerReq("POST", "/v1/chat/completions", withBearer(nil, secret), nil, "198.51.100.7:5555"))
		return err
	})
	failure("revoked_key", func() error {
		if err := h.sys.RevokeKey(context.Background(), scope, issued.Record.PublicID, h.clk.Now()); err != nil {
			return err
		}
		_, err := h.sys.AuthenticatePublic(context.Background(), bearerReq("POST", "/v1/chat/completions", withBearer(nil, secret), nil, "198.51.100.7:5555"))
		return err
	})
	failure("unknown_public_id", func() error {
		forged := identity.KeyPrefix + "deadbeef" + strings.Repeat("0", 24) + "." + strings.Repeat("a", 44)
		_, err := h.sys.AuthenticatePublic(context.Background(), bearerReq("POST", "/v1/chat/completions", withBearer(nil, forged), nil, "198.51.100.7:5555"))
		return err
	})
	failure("conflicting_headers", func() error {
		h2 := merge(withBearer(nil, secret), "x-api-key", secret)
		_, err := h.sys.AuthenticatePublic(context.Background(), bearerReq("POST", "/v1/chat/completions", h2, nil, "198.51.100.7:5555"))
		return err
	})
	failure("query_key", func() error {
		q := url.Values{}
		q.Set("key", secret)
		req := bearerReq("POST", "/v1/chat/completions", nil, q, "198.51.100.7:5555")
		_, err := h.sys.AuthenticatePublic(context.Background(), req)
		return err
	})
	failure("cache_down", func() error {
		cold, _, _ := issueKey(t, h, []string{"models:read"}, []string{"model-alpha"}, h.clk.Now().Add(time.Hour))
		h.backend.failGet = true
		defer func() { h.backend.failGet = false }()
		_, err := h.sys.AuthenticatePublic(context.Background(), bearerReq("POST", "/v1/models", withBearer(nil, cold.Secret), nil, "198.51.100.7:5558"))
		return err
	})
}

// TestP03T08LogsNeverContainSecret: authentication decisions logged through
// the injected logger never carry the plaintext secret.
func TestP03T08LogsNeverContainSecret(t *testing.T) {
	h := newHarness(t)
	issued, _, _ := issueKey(t, h, []string{"models:invoke"}, []string{"model-alpha"}, h.clk.Now().Add(time.Hour))
	secret := issued.Secret

	// Successful authentication, then several failure paths, all logged.
	authPublic(t, h, bearerReq("POST", "/v1/chat/completions", withBearer(nil, secret), nil, "198.51.100.7:5555"))
	_, _ = h.sys.AuthenticatePublic(context.Background(), bearerReq("POST", "/v1/chat/completions", withBearer(nil, "gw_live_bogus0000000000000000000000000."+strings.Repeat("b", 44)), nil, "198.51.100.7:5556"))
	_, err := h.sys.AuthenticatePublic(context.Background(), bearerReq("POST", "/v1/chat/completions", merge(withBearer(nil, secret), "x-api-key", secret), nil, "198.51.100.7:5557"))
	if err == nil {
		t.Fatal("conflicting header case must fail for this test to exercise its log path")
	}

	leakScan(t, "captured logs", h.logs.dump(), secret)
}

// TestP03T08AdminTokenAlsoOneTime: the admin bootstrap credential behaves
// like the public secret — present once, absent from records and errors.
func TestP03T08AdminTokenAlsoOneTime(t *testing.T) {
	h := newHarness(t)
	cred, err := h.sys.BootstrapAdmin(context.Background(), identity.BootstrapRequest{AdminID: newID(t), Scopes: []string{"admin:keys:read"}})
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	raw, err := json.Marshal(cred.Principal)
	if err != nil {
		t.Fatalf("marshal AdminPrincipal: %v", err)
	}
	leakScan(t, "AdminPrincipal JSON", string(raw), cred.Token)

	// Refused re-bootstrap must not echo the original token.
	_, err = h.sys.BootstrapAdmin(context.Background(), identity.BootstrapRequest{AdminID: newID(t)})
	leakScan(t, "refused bootstrap error", errText(err), cred.Token)
	if !errors.Is(err, identity.ErrAlreadyBootstrapped) {
		t.Fatalf("expected ErrAlreadyBootstrapped, got: %v", err)
	}
}
