package identity_test

// P03-T07 — conflicting or duplicated authentication headers must be rejected
// without choosing one, and any key material in the URL query must be
// rejected. Malformed Authorization schemes are plain rejections.

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"example.com/urbino/tests/identity"
)

// headerOf builds a single-pair header.
func headerOf(key, value string) http.Header {
	h := http.Header{}
	h.Set(key, value)
	return h
}

// merge returns a copy of h with additional key/value pairs appended
// (duplicates preserved) so duplicated-header cases are reproducible.
func merge(h http.Header, pairs ...string) http.Header {
	out := h.Clone()
	if out == nil {
		out = http.Header{}
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		out[pairs[i]] = append(out[pairs[i]], pairs[i+1])
	}
	return out
}

func TestP03T07ConflictingHeadersRejected(t *testing.T) {
	h := newHarness(t)
	issued, _, _ := issueKey(t, h, []string{"models:read"}, []string{"model-alpha"}, h.clk.Now().Add(time.Hour))
	other := syntheticSecret(t)

	cases := map[string]*identity.Request{
		"bearer_plus_x_api_key": {
			Method: "POST", Path: "/v1/chat/completions",
			Header: merge(withBearer(nil, issued.Secret), "x-api-key", issued.Secret),
			Query:  url.Values{}, RemoteAddr: "198.51.100.7:5555",
		},
		"bearer_plus_x_goog_api_key": {
			Method: "POST", Path: "/v1/chat/completions",
			Header: merge(withBearer(nil, issued.Secret), "x-goog-api-key", issued.Secret),
			Query:  url.Values{}, RemoteAddr: "198.51.100.7:5555",
		},
		"x_api_key_plus_x_goog_api_key": {
			Method: "POST", Path: "/v1/chat/completions",
			Header: merge(headerOf("x-api-key", issued.Secret), "x-goog-api-key", issued.Secret),
			Query:  url.Values{}, RemoteAddr: "198.51.100.7:5555",
		},
		"duplicate_authorization": {
			Method: "POST", Path: "/v1/chat/completions",
			Header: merge(withBearer(nil, issued.Secret), "Authorization", "Bearer "+issued.Secret),
			Query:  url.Values{}, RemoteAddr: "198.51.100.7:5555",
		},
		"duplicate_x_api_key": {
			Method: "POST", Path: "/v1/chat/completions",
			Header: merge(headerOf("x-api-key", issued.Secret), "x-api-key", issued.Secret),
			Query:  url.Values{}, RemoteAddr: "198.51.100.7:5555",
		},
		// Conflicting values: one valid, one garbage — the parser must not
		// gamble on the valid one.
		"bearer_valid_plus_x_api_key_garbage": {
			Method: "POST", Path: "/v1/chat/completions",
			Header: merge(withBearer(nil, issued.Secret), "x-api-key", other),
			Query:  url.Values{}, RemoteAddr: "198.51.100.7:5555",
		},
		"bearer_garbage_plus_x_api_key_valid": {
			Method: "POST", Path: "/v1/chat/completions",
			Header: merge(withBearer(nil, other), "x-api-key", issued.Secret),
			Query:  url.Values{}, RemoteAddr: "198.51.100.7:5555",
		},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			p, err := h.sys.AuthenticatePublic(context.Background(), req)
			if err == nil {
				t.Fatalf("conflicting credentials accepted: principal %+v", p)
			}
			if !errors.Is(err, identity.ErrAmbiguousCredentials) {
				t.Fatalf("expected ErrAmbiguousCredentials, got: %v", err)
			}
		})
	}
}

// TestP03T07QueryKeyRejected: any key material in the URL query is rejected
// even when headers are absent or clean.
func TestP03T07QueryKeyRejected(t *testing.T) {
	h := newHarness(t)
	issued, _, _ := issueKey(t, h, []string{"models:read"}, []string{"model-alpha"}, h.clk.Now().Add(time.Hour))

	for _, param := range []string{"key", "api_key", "apikey", "access_token", "token", "x-api-key", "x-goog-api-key"} {
		t.Run(param, func(t *testing.T) {
			q := url.Values{}
			q.Set(param, issued.Secret)
			req := bearerReq("POST", "/v1/chat/completions", nil, q, "198.51.100.7:5555")
			p, err := h.sys.AuthenticatePublic(context.Background(), req)
			if err == nil {
				t.Fatalf("query key in %q accepted: principal %+v", param, p)
			}
			if !errors.Is(err, identity.ErrQueryKey) {
				t.Fatalf("expected ErrQueryKey for %q, got: %v", param, err)
			}
			// Even alongside a valid header, the query parameter must
			// poison the request.
			req.Header = withBearer(nil, issued.Secret)
			if _, err := h.sys.AuthenticatePublic(context.Background(), req); !errors.Is(err, identity.ErrQueryKey) {
				t.Fatalf("query key must reject even with valid header, got: %v", err)
			}
		})
	}
}

// TestP03T07MalformedAuthorizationRejected: wrong schemes and garbage values
// are rejections, not crashes and not ambiguity.
func TestP03T07MalformedAuthorizationRejected(t *testing.T) {
	h := newHarness(t)
	issued, _, _ := issueKey(t, h, []string{"models:read"}, []string{"model-alpha"}, h.clk.Now().Add(time.Hour))

	cases := map[string]http.Header{
		"basic_scheme":    {"Authorization": {"Basic dXNlcjpwYXNz"}},
		"raw_no_scheme":   {"Authorization": {issued.Secret}},
		"empty_bearer":    {"Authorization": {"Bearer "}},
		"unknown_key":     {"Authorization": {"Bearer nothing"}},
		"whitespace_only": {"Authorization": {"   "}},
	}
	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			req := bearerReq("POST", "/v1/chat/completions", header, url.Values{}, "198.51.100.7:5555")
			p, err := h.sys.AuthenticatePublic(context.Background(), req)
			if err == nil {
				t.Fatalf("malformed authorization accepted: principal %+v", p)
			}
			if !errors.Is(err, identity.ErrUnauthorized) {
				t.Fatalf("expected unauthorized class, got: %v", err)
			}
		})
	}
}

// TestP03T07ConflictingHeadersRejectedOnAdminPath: the admin path applies the
// same ambiguity and query rules to admin tokens.
func TestP03T07ConflictingHeadersRejectedOnAdminPath(t *testing.T) {
	h := newHarness(t)
	cred, err := h.sys.BootstrapAdmin(context.Background(), identity.BootstrapRequest{AdminID: newID(t), Scopes: []string{"admin:keys:write"}})
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}

	cases := map[string]*identity.Request{
		"bearer_plus_x_api_key": {
			Method: "GET", Path: "/admin/v1/api-keys",
			Header: merge(withBearer(nil, cred.Token), "x-api-key", cred.Token),
			Query:  url.Values{}, RemoteAddr: "203.0.113.10:4444",
		},
		"query_key": {
			Method: "GET", Path: "/admin/v1/api-keys",
			Header: withBearer(nil, cred.Token),
			Query:  url.Values{"key": []string{cred.Token}}, RemoteAddr: "203.0.113.10:4444",
		},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			p, err := h.sys.AuthenticateAdmin(context.Background(), req)
			if err == nil {
				t.Fatalf("conflicting admin credentials accepted: principal %+v", p)
			}
			if !errors.Is(err, identity.ErrUnauthorized) {
				t.Fatalf("expected unauthorized class, got: %v", err)
			}
		})
	}
}

// TestP03T07CanonicalSingleHeadersAccepted: positive control proving the
// rejections above are about ambiguity, not about the headers themselves.
func TestP03T07CanonicalSingleHeadersAccepted(t *testing.T) {
	h := newHarness(t)
	issued, _, _ := issueKey(t, h, []string{"models:read"}, []string{"model-alpha"}, h.clk.Now().Add(time.Hour))

	for name, header := range map[string]http.Header{
		"bearer":         withBearer(nil, issued.Secret),
		"x_api_key":      headerOf("x-api-key", issued.Secret),
		"x_goog_api_key": headerOf("x-goog-api-key", issued.Secret),
	} {
		t.Run(name, func(t *testing.T) {
			req := bearerReq("POST", "/v1/chat/completions", header, url.Values{}, "198.51.100.7:5555")
			p := authPublic(t, h, req)
			if p.KeyID != issued.Record.PublicID {
				t.Fatalf("authenticated wrong key: got %q want %q", p.KeyID, issued.Record.PublicID)
			}
		})
	}
}
